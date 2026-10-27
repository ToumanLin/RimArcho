package gui

import (
	"encoding/json"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSortDependencyAPI(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	o, err := api.Run("sort")
	if err != nil || len(o.Refused) > 0 || len(o.MissingDependencies) != 1 {
		t.Fatalf("sort: %+v %v", o, err)
	}
	dep := o.MissingDependencies[0]
	if dep.Kind != "installed" || dep.Name != "Harmony" || strings.Join(dep.RequiredBy, ",") != "Alpha" {
		t.Fatalf("dep: %+v", dep)
	}
	raw, _ := json.Marshal(o)
	for _, field := range []string{`"missingDependencies"`, `"packageId"`, `"requiredBy"`, `"instance"`, `"ref"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("JSON lacks %s: %s", field, raw)
		}
	}
	before, _ := e.App.Loadout()
	r, err := api.CompleteDependencies()
	if err != nil || !r.Undo || len(r.Enabled) != 1 || len(r.MissingDependencies) != 0 || len(r.WorkshopIDs) != 0 {
		t.Fatalf("complete: %+v %v", r, err)
	}
	b := api.Board()
	if got := names(b.Cols[view.Active]); got != "Core,Harmony,Alpha" {
		t.Fatal(got)
	}
	if _, err := api.Undo(); err != nil {
		t.Fatal(err)
	}
	after, _ := e.App.Loadout()
	if !after.List.Equal(before.List) {
		t.Fatal("undo did not restore pre-completion state")
	}
}

func TestSortCycleOutcomeAndNeeds(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	a := testkit.Sample("tester.a", "Alpha")
	a.Deps = []string{"tester.b"}
	b := testkit.Sample("tester.b", "Beta")

	e.WriteMod(filepath.Join(e.Game, "Mods", "A"), a)
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), b)
	e.WriteFile(filepath.Join(e.Game, "Mods", "B", "About", "About.xml"), strings.Replace(b.AboutXML(), "</ModMetaData>", "<loadAfter><li>tester.a</li></loadAfter></ModMetaData>", 1))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a", "tester.b")
	e.AddEnv(false)
	e.MustRun("scan")
	api := &API{a: e.App}
	first, err := api.Run("sort")
	if err != nil || len(first.Refused) > 0 || len(first.Cycles) != 1 {
		t.Fatal(first, err)
	}
	cycle := first.Cycles[0]
	if len(cycle.Broken) != 1 || cycle.Broken[0].Kind != "loadAfter" || cycle.Broken[0].Before.PackageID != "tester.a" || cycle.Broken[0].After.PackageID != "tester.b" {
		t.Fatal(cycle)
	}
	second, err := api.Run("sort")
	if err != nil || second.Undo || !reflect.DeepEqual(first.Cycles, second.Cycles) {
		t.Fatalf("non-deterministic sort: %+v %v", second, err)
	}
	found := false
	for _, n := range api.needsLocked().Items {
		if !strings.Contains(n.Text, "cycle") {
			continue
		}
		found = true
		if !n.Dim || n.Counted || n.Status == view.StatusErr || !strings.Contains(n.Why, "Alpha") || !strings.Contains(n.Why, "Beta") || !strings.Contains(n.Why, "set aside") {
			t.Fatal(n)
		}
	}
	if !found {
		t.Fatal("cycle note absent from needs")
	}
	raw, _ := json.Marshal(first)
	if strings.Contains(string(raw), `"missingMods"`) || strings.Contains(string(raw), `"replacements"`) {
		t.Fatal("empty fields not omitted", string(raw))
	}
}

func TestReplaceRecommendedAPI(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Old"), testkit.Sample("tester.old", "Old"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "New"), testkit.Sample("tester.new", "New"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.old")
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[{"oldPackageId":"tester.old","newPackageId":"tester.new","newName":"New"}]}`)
	e.AddEnv(false)
	e.MustRun("scan")
	api := &API{a: e.App}
	out, err := api.Run("sort")
	if err != nil || len(out.Replacements) != 1 {
		t.Fatal(out, err)
	}
	before, _ := e.App.Loadout()
	r, err := api.ReplaceRecommended([]string{out.Replacements[0].Original.Ref})
	if err != nil || !r.Undo {
		t.Fatal(r, err)
	}
	if got := names(api.Board().Cols[view.Active]); got != "Core,New" {
		t.Fatal(got)
	}
	if _, err := api.Undo(); err != nil {
		t.Fatal(err)
	}
	restored, _ := e.App.Loadout()
	if !restored.List.Equal(before.List) {
		t.Fatal("replacement not undone")
	}
}
