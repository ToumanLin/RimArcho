package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The bytes the preview shows are the bytes that go; a relay without the route is quiet.
func TestSendTimings(t *testing.T) {
	var got string
	mux := http.NewServeMux()
	mux.HandleFunc("/timings", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		if !strings.HasPrefix(r.UserAgent(), "RimArcho/") {
			w.WriteHeader(403)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	tm := Timings{Version: "1.1.4", OS: "Windows 11", Game: "1.6", Active: 300, Total: 1300, WallMs: 48000, Phases: []TimingPhase{{"defs", 1200}}}
	if err := SendTimings(context.Background(), srv.URL+"/timings", "RimArcho/1.1.4", tm.Body()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tm.Body(), `"phases": [
    {"name": "defs", "ms": 1200}
  ]`) {
		t.Fatalf("body %s", tm.Body())
	}
	var back Timings
	if err := json.Unmarshal([]byte(tm.Body()), &back); err != nil || back.Phases[0].Ms != 1200 {
		t.Fatal(err, back)
	}
	if got != tm.Body() {
		t.Fatalf("sent %q, preview %q", got, tm.Body())
	}
	for _, banned := range []string{"name\": \"Mod", "path", "user", "steam"} {
		if strings.Contains(strings.ToLower(got), banned) {
			t.Fatalf("%q in %s", banned, got)
		}
	}
	err := SendTimings(context.Background(), srv.URL+"/nothing", "RimArcho/1.1.4", tm.Body())
	if !errors.Is(err, ErrNoTimingsRoute) {
		t.Fatalf("404: %v", err)
	}
	if err := SendTimings(context.Background(), "http://example.com/timings", "RimArcho/1", "{}"); err == nil {
		t.Fatal("plain http to another host")
	}
}

func TestTimingsURL(t *testing.T) {
	t.Setenv("RIMARCHO_FEEDBACK_RELAY", "http://127.0.0.1:9/report")
	if u := TimingsURL(); u != "http://127.0.0.1:9/timings" {
		t.Fatal(u)
	}
}
