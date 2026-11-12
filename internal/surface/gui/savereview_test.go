package gui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestReviewLayoutSettings(t *testing.T) {
	_, api, _ := fixture(t)
	for _, key := range []string{"ui.gui.review.left", "ui.gui.review.right", "ui.gui.review.detail"} {
		if err := api.SetSetting(key, "320"); err != nil {
			t.Fatalf("save %s: %v", key, err)
		}
		if got := api.Setting(key); got != "320" {
			t.Errorf("%s = %q, want 320", key, got)
		}
		if err := api.SetSetting(key, ""); err != nil {
			t.Fatalf("reset %s: %v", key, err)
		}
		if got := api.Setting(key); got != "" {
			t.Errorf("reset %s = %q", key, got)
		}
	}
}

func TestSaveReviewSelectionAndShell(t *testing.T) {
	e, api, _ := fixture(t)
	e.WriteSave("Old.rws", testkit.SaveSpec{Faction: "Old", Seed: "old", PRV: 1, ModTime: e.Now().Add(-2 * time.Hour)})
	e.WriteSave("New.rws", testkit.SaveSpec{Faction: "New", Seed: "new", PRV: 2, ModTime: e.Now().Add(-time.Hour)})
	var news []*ReviewEvent
	api.emit = func(name string, data any) {
		if name != "review" {
			return
		}
		ev := data.(*ReviewEvent)
		news = append(news, ev)
		if len(news) == 1 {
			var n int
			if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM save_cache`).Scan(&n); err != nil || n != 0 {
				t.Errorf("shell followed a full read: %d %v", n, err)
			}
		}
	}
	run := func(request uint64, colony string) *ReviewEvent {
		t.Helper()
		api.review.seq = request
		ev := api.runReview(request, colony)
		if ev == nil || ev.Error != "" || ev.Partial || ev.Page == nil {
			t.Fatalf("final: %+v", ev)
		}
		return ev
	}
	first := run(1, "")
	if len(news) == 0 || !news[0].Partial || news[0].Page.Done != 0 || news[0].Page.Total != 1 || len(news[0].Page.Groups) != 0 || len(news[0].Page.Colonies) != 2 || news[0].Page.All != "2 colonies, 2 saves" || news[0].Page.Lead == "" {
		t.Fatalf("shell: %+v", news)
	}
	if first.Page.Colony != first.Page.Colonies[0].Key || first.Page.Colonies[0].Name != "New" || first.Page.Done != 1 || first.Page.Total != 1 {
		t.Fatalf("default: %+v", first.Page)
	}
	if gone := run(2, "gone"); gone.Page != first.Page {
		t.Fatal("missing colony did not use kept default")
	}
	all := run(3, "*")
	if all.Page.Colony != "*" || all.Page.Done != 2 || all.Page.Total != 2 {
		t.Fatalf("all: %+v", all.Page)
	}
	old := run(4, all.Page.Colonies[1].Key)
	if old.Page.Colony != all.Page.Colonies[1].Key || old.Page.Total != 1 {
		t.Fatalf("specific: %+v", old.Page)
	}
}

func TestSaveReviewEmptyAndLeaveBeforeRead(t *testing.T) {
	e, api, _ := fixture(t)
	api.review.seq = 1
	empty := api.runReview(1, "")
	if empty == nil || empty.Page == nil || empty.Page.Colony != "" || empty.Page.Total != 0 || len(empty.Page.Groups) != 0 {
		t.Fatalf("empty: %+v", empty)
	}
	e.WriteSave("Fort.rws", testkit.SaveSpec{})
	api.emit = func(name string, data any) {
		if name == "review" {
			api.leaveReview()
		}
	}
	api.review.seq = 2
	if ev := api.runReview(2, ""); ev != nil {
		t.Fatalf("answered after leaving: %+v", ev)
	}
	var n int
	if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM save_cache`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("read after leaving: %d %v", n, err)
	}
}

// Saves › Play review (16 §2.11): a review reads its saves and answers with the page; the
// page is kept until a save changes; a newer request or leaving the page drops a run.
func TestSaveReviewAPI(t *testing.T) {
	e, api, events := fixture(t)
	e.AddWorkshop("3000000001", testkit.Mod{PackageID: "tester.hats", Name: "Hats", Files: map[string]string{
		"Defs/H.xml": "<Defs><ThingDef><defName>Cap</defName><category>Item</category><apparel><layers><li>Overhead</li></layers></apparel></ThingDef><ThingDef><defName>Helm</defName><category>Item</category><apparel><layers><li>Overhead</li></layers></apparel></ThingDef></Defs>\n"}})
	e.MustRun("scan")
	e.MustRun("analyze")
	mods := []string{"ludeon.rimworld", "tester.hats"}
	e.WriteSave("Fort.rws", testkit.SaveSpec{Mods: mods, Faction: "Fort", Seed: "alpha", PRV: 1, Body: `<things><li><def>Cap</def></li></things>`})

	run := func(request uint64, colony string) *ReviewEvent {
		t.Helper()
		api.review.mu.Lock()
		api.review.seq = request
		api.review.mu.Unlock()
		return api.runReview(request, colony)
	}
	ev := run(1, "")
	if ev == nil || ev.Error != "" || ev.Page == nil {
		t.Fatalf("review: %+v", ev)
	}
	p := ev.Page
	if len(p.Colonies) != 1 || p.Colonies[0].Name != "Fort" || p.Note != "" || !strings.Contains(p.Lead, "1 colony, 1 save") {
		t.Fatalf("page: %+v", p)
	}
	var hats *string
	for _, g := range p.Groups {
		for _, m := range g.Mods {
			if m.PackageID == "tester.hats" {
				if g.Key != "rest" || m.Size != 2 || m.Part != 1 || m.Seen != 1 || m.Adds != 2 || m.Sum != "1 of 2 seen" || m.Where != "1 colony, 1 save" || m.Library == "" ||
					len(m.Cats) != 1 || m.Cats[0].Label != "apparel" || strings.Join(m.Cats[0].Names, ",") != "Helm" {
					t.Errorf("hats: %s %+v", g.Key, m)
				}
				hats = &g.Key
			}
		}
	}
	if hats == nil {
		t.Fatalf("no Hats: %+v", p.Groups)
	}
	// the same saves: the kept page; a colony that is gone: the default colony
	if again := run(2, ""); again == nil || again.Page != p {
		t.Errorf("not kept: %+v", again)
	}
	if gone := run(3, "no-such-colony"); gone == nil || gone.Page != p || gone.Page.Colony != p.Colonies[0].Key {
		t.Errorf("gone colony: %+v", gone)
	}
	if one := run(4, p.Colonies[0].Key); one == nil || one.Page == nil || one.Page.Colony != p.Colonies[0].Key {
		t.Errorf("one colony: %+v", one)
	}
	// a new save: reviewed again, and the helmet is seen
	e.WriteSave("Fort 2.rws", testkit.SaveSpec{Mods: mods, Faction: "Fort", Seed: "alpha", PRV: 1, Body: `<things><li><def>Helm</def></li></things>`})
	next := run(5, "")
	if next == nil || next.Page == nil || next.Page == p {
		t.Fatalf("after a new save: %+v", next)
	}
	whole := false
	for _, g := range next.Page.Groups {
		for _, m := range g.Mods {
			whole = whole || g.Key == "rest" && m.PackageID == "tester.hats" && m.Seen == 2 && m.Adds == 2 && m.Part == 2 && m.Size == 2
		}
	}
	if !whole {
		t.Errorf("hats not all seen: %+v", next.Page.Groups)
	}
	// a request that is not the newest, and one after the page was left, answer nothing
	api.review.mu.Lock()
	api.review.seq = 9
	api.review.mu.Unlock()
	if api.runReview(8, "") != nil {
		t.Error("an older request answered")
	}
	api.leaveReview()
	if api.runReview(9, "") != nil {
		t.Error("answered after the page was left")
	}
	// SaveReviewRead answers by event
	for len(events) > 0 {
		<-events
	}
	if !api.SaveReviewRead(10, "") {
		t.Fatal("not started")
	}
	for {
		if ev := <-events; strings.HasPrefix(ev, "review ") {
			var result ReviewEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(ev, "review ")), &result); err != nil {
				t.Fatal(err)
			}
			if result.Partial {
				continue
			}
			if !strings.Contains(ev, `"request":10`) || !strings.Contains(ev, `"page"`) {
				t.Errorf("event: %s", ev)
			}
			break
		}
	}
}
