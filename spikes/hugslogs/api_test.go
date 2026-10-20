package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// A fake GitHub: 250 gists, one per hour going back from 2026-10-20 00:00, newest first,
// 100 per page; raw content is "log <id>".
func fakeGitHub(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	newest := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	pages := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/users/"+account+"/gists", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"message":"Bad credentials"}`, 401)
			return
		}
		pages++
		p, _ := strconv.Atoi(r.URL.Query().Get("page"))
		var out []map[string]any
		for i := (p - 1) * 100; i < p*100 && i < 250; i++ {
			out = append(out, map[string]any{
				"id":         fmt.Sprintf("%032x", i),
				"created_at": newest.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
				"files":      map[string]any{"output_log.txt": map[string]any{"filename": "output_log.txt", "size": 10}},
			})
		}
		if out == nil {
			out = []map[string]any{}
		}
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/"+account+"/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "log %s", filepath.Base(filepath.Dir(r.URL.Path)))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &pages
}

func TestAPIFetch(t *testing.T) {
	srv, pages := fakeGitHub(t)
	apiBase, rawBase, rawDelay = srv.URL, srv.URL, 0
	t.Setenv("GITHUB_TOKEN", "test-token")
	dir := t.TempDir()

	// From 2026-10-16 00:00: gists 0..96 are in range (96 hours back); the listing stops on page 1.
	if err := apiFetch(dir, "2026-10-16T00:00:00Z", 0, 2, 10); err != nil {
		t.Fatal(err)
	}
	idx, _ := readIndex(filepath.Join(dir, "index.jsonl"))
	if len(idx) != 97 || *pages != 1 {
		t.Fatalf("index %d entries after %d pages, want 97 after 1", len(idx), *pages)
	}
	raws, _ := os.ReadDir(filepath.Join(dir, "raw"))
	if len(raws) != 10 {
		t.Fatalf("%d raw logs, want 10 (-max)", len(raws))
	}
	// -max takes the newest first.
	if b, _ := os.ReadFile(rawPath(dir, fmt.Sprintf("%032x", 0))); string(b) != "log "+fmt.Sprintf("%032x", 0) {
		t.Fatalf("newest raw log missing or wrong: %q", b)
	}

	// A second run with no limit downloads the rest and adds nothing to the index.
	*pages = 0
	if err := apiFetch(dir, "2026-10-16T00:00:00Z", 0, 2, 0); err != nil {
		t.Fatal(err)
	}
	idx, _ = readIndex(filepath.Join(dir, "index.jsonl"))
	raws, _ = os.ReadDir(filepath.Join(dir, "raw"))
	if len(idx) != 97 || len(raws) != 97 {
		t.Fatalf("index %d, raw %d, want 97 and 97", len(idx), len(raws))
	}

	// No -from: 3 days before the newest indexed gist.
	f, err := resolveFrom(dir, "")
	if err != nil || f != "2026-10-17T00:00:00Z" {
		t.Fatalf("resolveFrom = %q, %v", f, err)
	}
	if f, _ := resolveFrom(dir, "2025-07-01"); f != "2025-07-01T00:00:00Z" {
		t.Fatalf("resolveFrom(date) = %q", f)
	}

	// The whole listing: 3 pages of gists, then an empty page.
	*pages = 0
	if err := apiFetch(dir, "2000-01-01T00:00:00Z", 0, 2, 0); err != nil {
		t.Fatal(err)
	}
	idx, _ = readIndex(filepath.Join(dir, "index.jsonl"))
	if len(idx) != 250 || *pages != 4 {
		t.Fatalf("index %d after %d pages, want 250 after 4", len(idx), *pages)
	}
}

func TestAPIBadToken(t *testing.T) {
	srv, _ := fakeGitHub(t)
	apiBase = srv.URL
	t.Setenv("GITHUB_TOKEN", "wrong")
	if _, err := apiPage("wrong", 1); err == nil {
		t.Fatal("bad token: want an error, not a retry loop")
	}
}
