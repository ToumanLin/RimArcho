package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// VM round 2 lane A (2026-11-23): a new, empty plan with three Mods enabled launched without
// a word; the game logged "Resetting mods config" and loaded Core alone.
func TestListWithoutCore(t *testing.T) {
	t.Parallel()
	view.SetLang("en")
	e := testkit.New(t)
	for i, id := range []string{"tester.a", "tester.b", "tester.c"} {
		e.AddWorkshop(string(rune('1'+i))+"001", testkit.Mod{PackageID: id, Name: strings.ToUpper(id[7:])})
	}
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("scan")
	e.MustRun("plan", "new", "r2a-del")
	e.MustRun("plan", "use", "r2a-del")
	e.MustRun("enable", "pkg:tester.a", "pkg:tester.b", "pkg:tester.c")

	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var core *app.Problem
	for _, p := range ps.List {
		if p.Code == app.CodeCoreMissing {
			core = p
		}
	}
	if core == nil || core.Severity != finding.Error || len(core.Suggestions) == 0 || core.Suggestions[0] != "archo sort" {
		t.Fatalf("finding %+v\nlist %s\n%s", core, enabled(t, e), e.MustRun("problems"))
	}

	// Can it run: one counted item that breaks the game, fixed by sorting.
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var item *view.Need
	for _, it := range view.BuildNeeds(e.App, st, ps).Items {
		if it.Title == "Core is not in the list" {
			item = &it
		}
	}
	if item == nil || !item.Counted || item.Status != view.StatusErr || view.SectionOf(item.Group) != view.SectionBreak ||
		len(item.Actions) != 1 || item.Actions[0].Kind != view.ActSort {
		t.Fatalf("item %+v", item)
	}

	// The CLI's launch preview lists it first, with sort as the fix.
	out := e.MustRun("launch", "--dry-run")
	if !strings.Contains(out, "error official.core-missing") || !strings.Contains(out, "fix: archo sort") {
		t.Fatalf("launch --dry-run:\n%s", out)
	}

	// The window asks before launching, and writes nothing while asking.
	file := e.ReadModsConfig()
	o := view.RunAsking(e.App, view.ListLaunch)
	if o.Text != "Core is not in the list" || len(o.Confirm) != 1 || o.Refused != nil || len(e.Launched) != 0 || e.ReadModsConfig() != file {
		t.Fatalf("question %+v, launched %v", o, e.Launched)
	}

	// Sort adds Core, first; the finding and the question go.
	if out := e.MustRun("sort"); !strings.Contains(out, "Core") {
		t.Fatalf("sort:\n%s", out)
	}
	if got := enabled(t, e); !strings.HasPrefix(got, "Core") {
		t.Fatalf("after sort: %s", got)
	}
	ps, err = e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps.List {
		if p.Code == app.CodeCoreMissing {
			t.Fatalf("still reported after sort: %+v", p)
		}
	}
	if o := view.RunAsking(e.App, view.ListLaunch); o.Confirm != nil || len(e.Launched) != 1 {
		t.Fatalf("asked after sort %+v", o)
	}
}
