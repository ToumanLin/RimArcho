package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A fake relay (feedback-relay/worker.mjs answers the same way): the report arrives as JSON
// with RimArcho's agent; the issue comes back; refusals keep the relay's code.
func TestSend(t *testing.T) {
	var got Report
	var agent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent = r.Header.Get("User-Agent")
		if r.Method != http.MethodPost || r.URL.Path != "/report" || r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		switch got.Title {
		case "fast":
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"slow_down"}`)
		case "broken":
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, `not json`)
		default:
			fmt.Fprint(w, `{"number":42,"url":"https://github.com/ToumanLin/RimArcho/issues/42"}`)
		}
	}))
	defer srv.Close()

	s, err := Send(context.Background(), srv.URL+"/report", "RimArcho/test", Report{Title: "t", Body: "b", Contact: "c"})
	if err != nil || s.Number != 42 || !strings.HasSuffix(s.URL, "/issues/42") {
		t.Fatalf("sent: %+v %v", s, err)
	}
	if got != (Report{Title: "t", Body: "b", Contact: "c"}) || agent != "RimArcho/test" {
		t.Fatalf("relay got %+v from %q", got, agent)
	}

	_, err = Send(context.Background(), srv.URL+"/report", "RimArcho/test", Report{Title: "fast"})
	var re *RelayError
	if !errors.As(err, &re) || re.Code != "slow_down" || re.Status != 429 {
		t.Fatalf("refusal: %v", err)
	}
	_, err = Send(context.Background(), srv.URL+"/report", "RimArcho/test", Report{Title: "broken"})
	if !errors.As(err, &re) || re.Status != 502 {
		t.Fatalf("a relay that fails: %v", err)
	}
	if _, err := Send(context.Background(), "http://example.com/report", "RimArcho/test", Report{}); err == nil {
		t.Fatal("plain http to another host is refused")
	}
}

func TestRelayURL(t *testing.T) {
	old := Relay
	defer func() { Relay = old }()
	Relay = ""
	t.Setenv("RIMARCHO_FEEDBACK_RELAY", "")
	if RelayURL() != "" {
		t.Fatal("no relay by default")
	}
	Relay = "https://relay.example/report"
	if RelayURL() != Relay {
		t.Fatal("the built-in address")
	}
	t.Setenv("RIMARCHO_FEEDBACK_RELAY", "https://test.example/report")
	if RelayURL() != "https://test.example/report" {
		t.Fatal("the variable overrides it")
	}
}

// A long mod list is cut from its end, says how many lines went, and the body fits.
func TestSendBody(t *testing.T) {
	d := Draft{Text: "Description", Basic: []string{"RimArcho dev"}, More: "… %d more lines"}
	if d.SendBody(MaxBody) != d.Body() {
		t.Fatal("a short body is sent whole")
	}
	var mods, log []string
	for i := 1; i <= 2000; i++ {
		mods = append(mods, fmt.Sprintf("%d. Mod %d  author.mod%d  %d", i, i, i, 1000000000+i))
	}
	for i := 1; i <= 40; i++ {
		log = append(log, fmt.Sprintf("E%d ×1  Could not resolve cross-reference", i))
	}
	d.Sections = []Section{{Heading: "mods", Lines: mods}, {Heading: "log", Lines: log}}
	b := d.SendBody(MaxBody)
	if units(b) > MaxBody || !strings.Contains(b, "1. Mod 1  ") || !strings.Contains(b, "E40 ×1") {
		t.Fatalf("fits, keeps the start of the list and the whole log: %d units", units(b))
	}
	if !strings.Contains(b, "more lines") || strings.Contains(b, "2000. Mod") || !strings.HasSuffix(b, "</details>\n") {
		t.Fatalf("says how many went, and the folds stay closed:\n%s", b[len(b)-300:])
	}
	if got := truncate("a😀b", 2); got != "a" {
		t.Fatalf("truncate keeps whole runes: %q", got)
	}
}
