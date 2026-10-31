package gui

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 设置 › 反馈 (06 §2.14): only the basics unless ticked; ticked parts are the list, the
// paths without the user's home, and the game log's error groups; the link stays openable.
func TestFeedbackAttachments(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	at, err := api.FeedbackAttach(view.FeedbackOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(at.Sections) != 0 || len(at.Basic) != 3 || !strings.HasPrefix(at.Basic[0], "RimArcho ") || at.Basic[1] != "RimWorld 1.6.4633 rev1261" {
		t.Fatalf("basics only by default: %+v", at)
	}
	if len(at.Parts) != 3 || at.Parts[0].Key != "mods" || at.Parts[0].None || !at.Parts[2].None {
		t.Fatalf("parts: %+v", at.Parts)
	}
	res := api.FeedbackCompose("标题", "说明")
	if !res.Full || strings.Contains(res.Body, "tester.a") || !strings.Contains(res.Body, "说明\n\n---\nRimArcho ") {
		t.Fatalf("basic body: %+v", res)
	}
	u, _ := url.Parse(res.URL)
	if !strings.HasPrefix(res.URL, feedback.Repo+"/issues/new?") || u.Query().Get("title") != "标题" || u.Query().Get("body") != res.Body {
		t.Fatalf("link: %s", res.URL)
	}

	at, err = api.FeedbackAttach(view.FeedbackOptions{Mods: true, Paths: true})
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
	if res := api.FeedbackCompose("t", ""); !strings.Contains(res.Body, "<summary>") || len(res.URL) > feedback.MaxURL {
		t.Fatalf("compose uses the attachments read last; the link stays short: %d", len(res.URL))
	}

	log, err := os.ReadFile(filepath.Join("..", "..", "logs", "testdata", "synthetic-save-loading.log"))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(e.UserData, "Player.log"), string(log))
	deadline := time.Now().Add(30 * time.Second)
	for {
		at, err = api.FeedbackAttach(view.FeedbackOptions{Log: true})
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
