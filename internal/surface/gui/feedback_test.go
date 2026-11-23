//go:build windows || cgo

package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// other is a report of the "other / help" template with the given attachments and words.
func other(o view.FeedbackOptions, title, text string) view.FeedbackProblem {
	return view.FeedbackProblem{Kind: feedback.KindOther, Title: title, Answers: map[string]string{"text": text}, Opts: o}
}

// Settings > Feedback (16 §2.14): only the basics unless ticked; ticked parts are the list, the
// paths without the user's home, and the game log's error groups; the link stays openable.
func TestFeedbackAttachments(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	at, err := api.FeedbackAttach(other(view.FeedbackOptions{}, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(at.Sections) != 0 || len(at.Basic) != 4 || at.Basic[0] != "report: other / help" || !strings.HasPrefix(at.Basic[1], "RimArcho ") || at.Basic[2] != "RimWorld 1.6.4633 rev1261" {
		t.Fatalf("basics only by default: %+v", at)
	}
	if len(at.Parts) != 5 || at.Parts[4].Key != "problems" || !at.Parts[4].None || at.Parts[0].Key != "mods" || at.Parts[0].None || !at.Parts[2].None || at.Parts[3].Key != "ops" || !at.Parts[3].None {
		t.Fatalf("parts: %+v", at.Parts)
	}
	res := api.FeedbackCompose(other(view.FeedbackOptions{}, "Title", "Description"))
	if !res.Full || strings.Contains(res.Body, "tester.a") || !strings.Contains(res.Body, "Description\n\n---\nreport: other / help\nRimArcho ") {
		t.Fatalf("basic body: %+v", res)
	}
	u, _ := url.Parse(res.URL)
	if !strings.HasPrefix(res.URL, feedback.Repo+"/issues/new?") || u.Query().Get("title") != "Title" || u.Query().Get("body") != res.Body {
		t.Fatalf("link: %s", res.URL)
	}

	at, err = api.FeedbackAttach(other(view.FeedbackOptions{Mods: true, Paths: true}, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(at.Sections) != 2 {
		t.Fatalf("mods and paths: %+v", at.Sections)
	}
	mods, paths := strings.Join(at.Sections[0].Lines, "\n"), strings.Join(at.Sections[1].Lines, "\n")
	if !strings.Contains(mods, "1. Core  ludeon.rimworld") || !strings.Contains(mods, "2. Alpha  tester.a  1001") {
		t.Fatalf("mods in load order:\n%s", mods)
	}
	if !strings.Contains(paths, "game: ") || !strings.Contains(paths, "workshop: ") {
		t.Fatalf("paths:\n%s", paths)
	}
	if home, _ := os.UserHomeDir(); home != "" && strings.Contains(strings.ToLower(paths), strings.ToLower(home)) {
		t.Fatalf("the user's home stays out:\n%s", paths)
	}
	if res := api.FeedbackCompose(other(view.FeedbackOptions{}, "t", "")); !strings.Contains(res.Body, "<summary>") || len(res.URL) > feedback.MaxURL {
		t.Fatalf("compose uses the attachments read last; the link stays short: %d", len(res.URL))
	}
	// Recent operations (13 §1.3): off unless ticked, the CLI's text, the home scrubbed.
	home, _ := os.UserHomeDir()
	e.App.Ops.Add(oplog.Row{Op: oplog.Trash, Subject: "Old Mod", Target: filepath.Join(home, "Mods", "Old Mod"), Result: oplog.Refused, Detail: "not on the list"})
	if at, _ := api.FeedbackAttach(other(view.FeedbackOptions{}, "", "")); at.Parts[3].None || len(at.Sections) != 0 {
		t.Fatalf("ops part: %+v", at.Parts[3])
	}
	at, _ = api.FeedbackAttach(other(view.FeedbackOptions{Ops: true}, "", ""))
	if len(at.Sections) != 1 || at.Sections[0].Key != "ops" {
		t.Fatalf("ops: %+v", at.Sections)
	}
	opsText := strings.Join(at.Sections[0].Lines, "\n")
	if !strings.Contains(opsText, "trash") || !strings.Contains(opsText, "refused") || !strings.Contains(opsText, "Old Mod") ||
		home != "" && strings.Contains(strings.ToLower(opsText), strings.ToLower(home)) {
		t.Fatalf("ops text:\n%s", opsText)
	}

	log, err := os.ReadFile(filepath.Join("..", "..", "logs", "testdata", "synthetic-save-loading.log"))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(e.UserData, "Player.log"), string(log))
	deadline := time.Now().Add(30 * time.Second)
	for {
		at, err = api.FeedbackAttach(other(view.FeedbackOptions{Log: true}, "", ""))
		if err != nil {
			t.Fatal(err)
		}
		if !at.Pending || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if at.Pending || len(at.Sections) != 1 || !strings.Contains(strings.Join(at.Sections[0].Lines, "\n"), "SyntheticMissingDef") {
		t.Fatalf("game log errors: pending=%v %+v", at.Pending, at.Sections)
	}
}

// send (16 §2.14): offered only with a relay; the report carries the title (or the first line
// of what happened), the ticked attachments and the contact; the relay's refusal is said in
// the player's words. The relay is httptest, standing in for feedback-relay/.
func TestFeedbackSend(t *testing.T) {
	view.SetLang("en")
	_, api, _ := fixture(t)
	old := feedback.Relay
	defer func() { feedback.Relay = old }()
	feedback.Relay = ""
	t.Setenv("RIMARCHO_FEEDBACK_RELAY", "")
	if at, err := api.FeedbackAttach(other(view.FeedbackOptions{}, "", "")); err != nil || at.Send {
		t.Fatalf("no relay, no Send: %+v %v", at, err)
	}

	var got feedback.Report
	var agent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent = r.Header.Get("User-Agent")
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got.Title == "again" {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"slow_down"}`)
			return
		}
		fmt.Fprint(w, `{"number":7,"url":"https://github.com/ToumanLin/RimArcho/issues/7"}`)
	}))
	defer srv.Close()
	t.Setenv("RIMARCHO_FEEDBACK_RELAY", srv.URL+"/report")

	at, err := api.FeedbackAttach(other(view.FeedbackOptions{Mods: true}, "", ""))
	if err != nil || !at.Send {
		t.Fatalf("a relay offers Send: %+v %v", at, err)
	}
	if _, err := api.FeedbackSend([]view.FeedbackProblem{other(view.FeedbackOptions{}, " ", "\n")}, ""); err == nil {
		t.Fatal("nothing written, nothing sent")
	}
	ss, err := api.FeedbackSend([]view.FeedbackProblem{other(view.FeedbackOptions{Mods: true}, "", "Mods vanish\nafter saving")}, "me#1234")
	if err != nil || len(ss) != 1 || ss[0].Number != 7 {
		t.Fatalf("sent: %+v %v", ss, err)
	}
	if got.Title != "Mods vanish" || got.Contact != "me#1234" || !strings.HasPrefix(agent, "RimArcho/") {
		t.Fatalf("report: %+v from %q", got, agent)
	}
	if got.Kind != "other" || got.Priority || !strings.HasPrefix(got.Body, "Mods vanish\nafter saving\n\n---\nreport: other / help\nRimArcho ") || !strings.Contains(got.Body, "2. Alpha  tester.a  1001") || strings.Contains(got.Body, "me#1234") {
		t.Fatalf("body: the text, the basics and the ticked list, not the contact:\n%s", got.Body)
	}
	if _, err := api.FeedbackSend([]view.FeedbackProblem{other(view.FeedbackOptions{}, "again", "")}, ""); err == nil || !strings.Contains(err.Error(), "one report a minute") {
		t.Fatalf("refusal in words: %v", err)
	}
}

// One report, one topic (16 §2.14): two problems go in one request and come back as two
// issues; a bug whose files were changed or lost carries the recent operations and asks for
// the priority label; each problem has its own template's words and attachments.
func TestFeedbackSplitAndPriority(t *testing.T) {
	view.SetLang("en")
	e, api, _ := fixture(t)
	e.App.Ops.Add(oplog.Row{Op: oplog.Trash, Subject: "Old Mod", Target: "C:/x/Old Mod", Result: oplog.OK})
	var got struct {
		Reports []feedback.Report `json:"reports"`
		Contact string            `json:"contact"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, `{"number":8,"url":"u8","token":"t8xxxxxxxxxxxxxxxxxx","issues":[{"number":8,"url":"u8","token":"t8xxxxxxxxxxxxxxxxxx"},{"number":9,"url":"u9","token":"t9xxxxxxxxxxxxxxxxxx"}]}`)
	}))
	defer srv.Close()
	t.Setenv("RIMARCHO_FEEDBACK_RELAY", srv.URL+"/report")
	bug := view.FeedbackProblem{Kind: feedback.KindBug, Answers: map[string]string{"did": "Removed a subscription", "actual": "Another folder went too", "files": "yes"}}
	idea := view.FeedbackProblem{Kind: feedback.KindFeature, Title: "Size column", Answers: map[string]string{"want": "Sort by size"}}
	at, err := api.FeedbackAttach(bug)
	if err != nil || len(at.Sections) != 1 || at.Sections[0].Key != "ops" || at.Basic[0] != "report: bug" {
		t.Fatalf("files lost ticks the operations: %+v %v", at, err)
	}
	ss, err := api.FeedbackSend([]view.FeedbackProblem{bug, idea}, "me")
	if err != nil || len(ss) != 2 || ss[1].Number != 9 {
		t.Fatalf("two issues: %+v %v", ss, err)
	}
	if len(got.Reports) != 2 || got.Contact != "me" || got.Reports[0].Contact != "" {
		t.Fatalf("one request, one contact: %+v", got)
	}
	b0, b1 := got.Reports[0], got.Reports[1]
	if b0.Kind != "bug" || !b0.Priority || b0.Title != "Removed a subscription" || !strings.HasPrefix(b0.Body, "**files were changed or lost**\n\n**what you did**\nRemoved a subscription") ||
		!strings.Contains(b0.Body, "recent operations") || !strings.Contains(b0.Body, "Old Mod") {
		t.Fatalf("bug report:\n%+v", b0)
	}
	if b1.Kind != "feature" || b1.Priority || b1.Title != "Size column" || strings.Contains(b1.Body, "Old Mod") || !strings.Contains(b1.Body, "**what you want to do**\nSort by size") {
		t.Fatalf("feature report:\n%+v", b1)
	}
	if rs := view.ReadFeedbackReports(e.App); len(rs.Reports) != 2 {
		t.Fatalf("both recorded for replies: %+v", rs.Reports)
	}
}
