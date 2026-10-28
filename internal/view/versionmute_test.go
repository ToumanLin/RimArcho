package view_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestVersionReasonsAndMuteActions(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.5000 rev1")
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.a", Name: "Alpha", Supported: []string{"1.6"}})
	e.AddWorkshop("1002", testkit.Mod{PackageID: "tester.b", Name: "Beta", Supported: []string{"1.5", "1.6"}})
	e.WriteModsConfig("1.7.5000 rev1", "ludeon.rimworld", "tester.a", "tester.b")
	e.AddEnv(false)
	e.MustRun("scan")
	build := func() (*view.Needs, *app.ProblemSet) {
		t.Helper()
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			t.Fatal(err)
		}
		return view.BuildNeeds(e.App, ps.State, ps), ps
	}
	n, ps := build()
	var merged *view.Need
	for i := range n.Items {
		if strings.Contains(n.Items[i].Text, "mods do not list 1.7") {
			merged = &n.Items[i]
		}
	}
	if merged == nil || !merged.Counted || len(merged.Mods) != 2 {
		t.Fatalf("merged: %+v", merged)
	}
	batch := merged.Actions[1]
	if batch.Kind != view.ActVersionMute || batch.Confirm == "" || batch.Warn == "" || len(batch.Refs) != 2 {
		t.Fatalf("batch: %+v", batch)
	}
	if merged.Actions[0].Kind != view.ActVersionList || len(merged.Actions[0].Entries) != 2 {
		t.Fatalf("list: %+v", merged.Actions)
	}
	var alpha string
	for id, rs := range n.Reasons {
		m := ps.State.Ix.ByID[id]
		if len(rs) != 1 {
			t.Fatalf("%s reasons: %+v", m.Name, rs)
		}
		r := rs[0]
		if strings.Contains(r.Text, "mods") || !strings.Contains(r.Text, "does not list 1.7") || len(r.Actions) != 1 || r.Actions[0].Kind != view.ActVersionMute || r.Actions[0].Confirm != "" {
			t.Fatalf("row reason: %+v", r)
		}
		if m.Name == "Alpha" {
			alpha = id
			if !strings.HasSuffix(r.Text, "(lists 1.6)") {
				t.Fatal(r.Text)
			}
		}
		if m.Name == "Beta" && !strings.HasSuffix(r.Text, "(lists 1.5, 1.6)") {
			t.Fatal(r.Text)
		}
	}
	if alpha == "" {
		t.Fatal("missing Alpha reason")
	}
	// Event payload carries these individual reasons, so JS never recreates merged ones.
	b, err := json.Marshal(view.ProblemsOf(n))
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Reasons map[string][]view.Reason `json:"reasons"`
	}
	if err := json.Unmarshal(b, &event); err != nil || event.Reasons[alpha][0].Text != n.Reasons[alpha][0].Text {
		t.Fatalf("event reasons: %s, %v", b, err)
	}
	if _, err := view.Do(e.App, n.Reasons[alpha][0].Actions[0], 0); err != nil {
		t.Fatal(err)
	}
	n, ps = build()
	if len(ps.Muted) != 1 || n.Status[alpha] != 0 || len(n.Reasons[alpha]) != 0 {
		t.Fatal("muted row still warned")
	}
	r, err := view.VersionReason(e.App, ps.State, ps.State.Ix.ByID[alpha])
	if err != nil || r == nil || r.Status != view.StatusNone || len(r.Actions) != 1 || r.Actions[0].Kind != view.ActVersionUnmute {
		t.Fatalf("muted detail: %+v %v", r, err)
	}
	if _, err := view.Do(e.App, batch, 0); err != nil {
		t.Fatal(err)
	}
	n, ps = build()
	if n.Count != 0 || len(n.Status) != 0 || len(ps.Muted) != 2 {
		t.Fatalf("all muted: %+v", n)
	}
	var muted *view.Need
	for i := range n.Items {
		if n.Items[i].Text == "2 mods muted" {
			muted = &n.Items[i]
		}
	}
	if muted == nil || !muted.Dim || muted.Counted || len(muted.Actions[0].Entries) != 2 {
		t.Fatalf("muted line: %+v", muted)
	}
	if _, err := view.Do(e.App, muted.Actions[0].Entries[0].Action, 0); err != nil {
		t.Fatal(err)
	}
	n, ps = build()
	if len(ps.Muted) != 1 || n.Count != 1 {
		t.Fatalf("restored counts: muted=%d count=%d", len(ps.Muted), n.Count)
	}
	// A later update can declare support; the stored choice still has an undo in detail.
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.a", Name: "Alpha", Supported: []string{"1.7"}})
	e.MustRun("scan")
	e.MustRun("problems", "mute-version", "pkg:tester.a")
	_, ps = build()
	for _, m := range ps.State.Ix.Mods {
		if m.Name != "Alpha" {
			continue
		}
		r, err := view.VersionReason(e.App, ps.State, m)
		if err != nil || r == nil || r.Actions[0].Kind != view.ActVersionUnmute {
			t.Fatalf("updated detail: %+v %v", r, err)
		}
	}
}

func TestVersionReasonChinese(t *testing.T) {
	view.SetLang("zh")
	t.Cleanup(func() { view.SetLang("en") })
	if got := view.Tf("does not list %s as supported (lists %s)", "1.7", "1.6"); got != "没有标明支持 1.7（标明了 1.6）" {
		t.Fatal(got)
	}
}
