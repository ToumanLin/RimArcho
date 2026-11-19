package view_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// relay is a fake feedback relay's status endpoint: issue 7 (token "tok7") has one owner
// reply; anything else is 404. down makes it answer 502.
func relay(t *testing.T, down *atomic.Bool, asked *atomic.Int32) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		if !strings.HasPrefix(r.UserAgent(), "RimArcho/") {
			w.WriteHeader(403)
			return
		}
		if down.Load() {
			w.WriteHeader(502)
			return
		}
		if r.URL.Path != "/status/7" || r.URL.Query().Get("t") != "tok7" {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "open", "closed_at": nil,
			"replies": []map[string]any{{"body": "<b>thanks</b>, fixed", "created_at": "2026-11-10T00:00:00Z"}}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/report"
}

func TestFeedbackRepliesRecordCheckSeen(t *testing.T) {
	e := testkit.New(t)
	var down atomic.Bool
	var asked atomic.Int32
	t.Setenv("RIMARCHO_FEEDBACK_RELAY", relay(t, &down, &asked))

	if r := view.CheckFeedbackReplies(context.Background(), e.App); len(r.Reports) != 0 || r.Unseen != 0 || !r.Checking {
		t.Fatalf("no reports yet: %+v", r)
	}
	view.RecordFeedbackSent(e.App, feedback.Sent{Number: 7, URL: "https://github.com/x/y/issues/7", Token: "tok7"}, "Crash")
	view.RecordFeedbackSent(e.App, feedback.Sent{Number: 8, Token: "tok8"}, "Other")
	view.RecordFeedbackSent(e.App, feedback.Sent{Number: 9}, "no token: a relay before replies")

	b, err := os.ReadFile(filepath.Join(e.App.Home, feedback.ReportsFile))
	if err != nil || !strings.Contains(string(b), `"token": "tok7"`) || !strings.Contains(string(b), `"sent_at"`) || !strings.Contains(string(b), `"last_seen_reply"`) {
		t.Fatalf("the record: %s %v", b, err)
	}

	r := view.CheckFeedbackReplies(context.Background(), e.App)
	if len(r.Reports) != 2 || r.Unseen != 1 {
		t.Fatalf("after a check: %+v", r)
	}
	for _, p := range r.Reports {
		switch p.Number {
		case 7:
			if !p.Unseen || len(p.Replies) != 1 || p.Replies[0].Body != "<b>thanks</b>, fixed" || p.State != "open" {
				t.Fatalf("#7: %+v", p)
			}
		case 8:
			if !p.Gone || p.Unseen {
				t.Fatalf("#8 is unknown to the relay: %+v", p)
			}
		}
	}

	view.SeenFeedbackReplies(e.App)
	if r := view.ReadFeedbackReports(e.App); r.Unseen != 0 {
		t.Fatalf("seen: %+v", r)
	}

	// relay down: quiet, the last answer stays, no dot
	down.Store(true)
	r = view.CheckFeedbackReplies(context.Background(), e.App)
	if !r.Failed || r.Unseen != 0 || len(r.Reports[1].Replies)+len(r.Reports[0].Replies) != 1 {
		t.Fatalf("relay down: %+v", r)
	}
	down.Store(false)

	// checking off: nothing is asked
	e.MustRun("config", "set", "feedback.replies", "off")
	n := asked.Load()
	r = view.CheckFeedbackReplies(context.Background(), e.App)
	if asked.Load() != n || r.Checking || !r.Off {
		t.Fatalf("checking off asked the relay: %+v", r)
	}
	e.MustRun("config", "set", "feedback.replies", "on")

	// no relay: nothing is asked, no dot
	t.Setenv("RIMARCHO_FEEDBACK_RELAY", "")
	old := feedback.Relay
	feedback.Relay = ""
	t.Cleanup(func() { feedback.Relay = old })
	if r := view.CheckFeedbackReplies(context.Background(), e.App); r.Checking || r.Unseen != 0 || asked.Load() != n {
		t.Fatalf("no relay: %+v", r)
	}
}

func TestFeedbackRecordDue(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	closed := now.Add(-31 * 24 * time.Hour)
	recent := now.Add(-29 * 24 * time.Hour)
	for _, c := range []struct {
		r    feedback.Record
		want bool
	}{
		{feedback.Record{Token: "t", State: "open"}, true},
		{feedback.Record{Token: "t", State: "closed", ClosedAt: &recent}, true},
		{feedback.Record{Token: "t", State: "closed", ClosedAt: &closed}, false},
		{feedback.Record{Token: "t", Gone: true}, false},
		{feedback.Record{State: "open"}, false},
	} {
		if got := c.r.Due(now); got != c.want {
			t.Errorf("%+v: due %v, want %v", c.r, got, c.want)
		}
	}
	if u := feedback.StatusURL("https://relay.example/report", 41, "a b"); u != "https://relay.example/status/41?t=a+b" {
		t.Errorf("status URL %q", u)
	}
	if feedback.StatusURL("https://relay.example/other", 41, "t") != "" {
		t.Error("a relay address without /report has no status")
	}
}
