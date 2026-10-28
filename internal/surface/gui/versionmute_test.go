package gui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestVersionMuteRefreshesBoardAndDetail(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.5000 rev1")
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.a", Name: "Alpha", Supported: []string{"1.6"}})
	e.WriteModsConfig("1.7.5000 rev1", "ludeon.rimworld", "tester.a")
	e.AddEnv(false)
	e.MustRun("scan")
	a := NewAPI(e.App, func(string, any) {})
	t.Cleanup(a.Close)
	b := a.Board()
	var id string
	for k, rs := range b.Reasons {
		if len(rs) > 0 && strings.Contains(rs[0].Text, "does not list 1.7") {
			id = k
		}
	}
	if id == "" {
		t.Fatalf("no version reason: %+v", b.Reasons)
	}
	d, err := a.Detail(id, false, false, false)
	if err != nil || d.Version == nil || d.Version.Actions[0].Kind != view.ActVersionMute {
		t.Fatalf("detail: %+v %v", d, err)
	}
	panel, err := a.VersionWarnings("1.7", false)
	if err != nil || len(panel.Groups) != 1 || panel.Groups[0].Rows[0].ID != id {
		t.Fatalf("panel: %+v %v", panel, err)
	}
	if _, err := a.DoAction(b.Reasons[id][0].Actions[0], 0); err != nil {
		t.Fatal(err)
	}
	p := a.ListProblems()
	if p.Count != 0 || p.Status[id] != 0 || len(p.Reasons[id]) != 0 {
		t.Fatalf("stale event: %+v", p)
	}
	panel, err = a.VersionWarnings("1.7", false)
	if err != nil || panel.Groups == nil || len(panel.Groups) != 0 {
		t.Fatalf("empty panel: %+v %v", panel, err)
	}
	panel, err = a.VersionWarnings("1.7", true)
	if err != nil || len(panel.Groups) != 1 || len(panel.All.Refs) != 1 || panel.All.Kind != view.ActVersionUnmute {
		t.Fatalf("ignored panel: %+v %v", panel, err)
	}
	d, err = a.Detail(id, false, false, false)
	if err != nil || d.Version == nil || d.Version.Actions[0].Kind != view.ActVersionUnmute {
		t.Fatalf("muted detail: %+v %v", d, err)
	}
	if _, err := a.DoAction(d.Version.Actions[0], 0); err != nil {
		t.Fatal(err)
	}
	if p := a.ListProblems(); p.Count != 1 || len(p.Reasons[id]) != 1 {
		t.Fatalf("restored event: %+v", p)
	}
}
