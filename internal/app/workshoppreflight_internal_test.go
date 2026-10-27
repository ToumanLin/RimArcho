package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type workshopHTTP func(*http.Request) (*http.Response, error)

func (f workshopHTTP) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWorkshopPreflightChunksAndFallback(t *testing.T) {
	var counts []int
	a := &App{HTTP: &http.Client{Transport: workshopHTTP(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != SteamAPI {
			t.Fatalf("request %s %s", r.Method, r.URL)
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 3*time.Second {
			t.Fatal("preflight lacks a short timeout")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		n, _ := strconv.Atoi(r.Form.Get("itemcount"))
		counts = append(counts, n)
		var rows []string
		for i := 0; i < n; i++ {
			id := r.Form.Get(fmt.Sprintf("publishedfileids[%d]", i))
			rows = append(rows, fmt.Sprintf(`{"publishedfileid":%q,"result":1,"consumer_app_id":294100,"title":"Real title","file_size":"900"}`, id))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"response":{"publishedfiledetails":[` + strings.Join(rows, ",") + `]}}`))}, nil
	})}}
	var ids []string
	for i := 1; i <= 205; i++ {
		ids = append(ids, strconv.Itoa(i))
	}
	details := a.preflightWorkshop(context.Background(), ids)
	if fmt.Sprint(counts) != "[100 100 5]" || len(details) != 205 || details["1"].Title != "Real title" {
		t.Fatalf("counts=%v details=%v", counts, details)
	}
	for _, tc := range []struct {
		name, body string
		status     int
		err        error
	}{
		{"offline", "", 0, errors.New("offline")},
		{"status", `{}`, 503, nil},
		{"bad JSON", `{`, 200, nil},
		{"missing response", `{}`, 200, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.HTTP.Transport = workshopHTTP(func(*http.Request) (*http.Response, error) {
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			if got := a.preflightWorkshop(context.Background(), ids); len(got) != 0 {
				t.Fatalf("failure classified items: %+v", got)
			}
		})
	}
}

func TestWorkshopMissingClassification(t *testing.T) {
	for _, reason := range []string{"File Not Found", "FileNotFound", "Private", "Not visible"} {
		r := parseWorkshopOutput([]string{"8100"}, "ERROR! Download item 8100 failed ("+reason+").", nil)["8100"]
		if r.kind != "missing" || !workshopPermanent(r.kind) || strings.Contains(r.why(), "account") {
			t.Fatalf("%s: %+v %s", reason, r, r.why())
		}
	}
	if !workshopPermanent("disk") || !workshopPermanent("ownership") || workshopPermanent("network") {
		t.Fatal("retry classification")
	}
}
