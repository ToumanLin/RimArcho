package app_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestSortMissingModProvenance(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	dir := e.AddWorkshop("54321", testkit.Sample("tester.old", "Recorded old name"))
	scanNow(t, e)
	old := one(t, e, "Recorded old name")
	e.Move(dir, filepath.Join(e.Root, "removed"))
	scanNow(t, e)
	e.WriteFile(filepath.Join(e.Home, "cache", "steamDB.index.json"), `{"tester.cached":["12345","Cached title"]}`)
	il := &app.ImportedList{Tokens: []string{"TESTER.ENTRY_STEAM", "tester.renamed", "tester.cached", "tester.unknown"},
		Names: map[string]string{"tester.entry_steam": "Imported title"}, Workshop: map[string]string{"tester.entry_steam": "98765"},
		Lineages: map[string]string{"tester.renamed": old.LineageID}}
	if _, err := e.App.ApplyImport(il, "provenance", false); err != nil {
		t.Fatal(err)
	}
	r, err := e.App.Sort(false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]app.SortMod{}
	for _, m := range r.MissingMods {
		got[m.PackageID] = m.SortMod
	}
	if len(got) != 4 {
		t.Fatal(got)
	}
	for pkg, want := range map[string][2]string{"tester.entry": {"98765", "Imported title"}, "tester.renamed": {"54321", "Recorded old name"}, "tester.cached": {"12345", "Cached title"}, "tester.unknown": {"", "tester.unknown"}} {
		m := got[pkg]
		if m.WorkshopID != want[0] || m.Name != want[1] {
			t.Fatalf("%s: %+v", pkg, m)
		}
	}
	data, err := e.App.ExportLoadout(app.FormatArcho)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := app.ParseList(data)
	if err != nil || parsed.Workshop["tester.entry"] != "98765" || parsed.Lineages["tester.renamed"] != old.LineageID {
		t.Fatalf("metadata lost on export: %s %v", data, err)
	}
	stored, err := e.App.Store.Workspace(r.State.Plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	decoded := loadout.Decode(stored.Entries)
	if len(decoded) < 4 || decoded[1].Name == "" {
		t.Fatal("metadata lost on disk", stored.Entries)
	}
}

func TestSortReplacementsAndAtomicUndo(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	for _, m := range []testkit.Mod{
		{PackageID: "tester.a", Name: "Old A"}, {PackageID: "tester.b", Name: "Old B"},
		{PackageID: "tester.new", Name: "New"}, {PackageID: "tester.remote", Name: "Remote old"},
		{PackageID: "tester.other", Name: "Wrong game"},
	} {
		e.WriteMod(filepath.Join(e.Game, "Mods", m.Name), m)
	}
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a", "tester.b", "tester.remote", "tester.other")
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[
 {"oldPackageId":"tester.a","newPackageId":"tester.new","newName":"New","newWorkshopId":"8001","newVersions":["1.6"]},
 {"oldPackageId":"tester.b","newPackageId":"tester.new","newName":"New","newWorkshopId":"8001"},
 {"oldPackageId":"tester.remote","newPackageId":"tester.notinstalled","newName":"Remote new","newWorkshopId":"8002"},
 {"oldPackageId":"tester.other","newPackageId":"tester.badversion","newName":"Wrong game","newVersions":["1.7"]}]}`)
	scanNow(t, e)
	r, err := e.App.Sort(false)
	if err != nil || len(r.Replacements) != 3 {
		t.Fatalf("sort: %+v %v", r, err)
	}
	byPkg := map[string]app.ReplacementSuggestion{}
	for _, s := range r.Replacements {
		byPkg[s.Original.PackageID] = s
	}
	if !byPkg["tester.a"].Installed || !byPkg["tester.a"].CanReplace || byPkg["tester.remote"].Installed || byPkg["tester.remote"].Replacement.WorkshopID != "8002" {
		t.Fatal(byPkg)
	}
	before := r.State.List.Clone()
	undoHead := e.App.Store.LastStepID(r.State.Plan.ID, "undo")
	if _, err := e.App.ReplaceRecommended([]string{byPkg["tester.a"].Original.Ref, byPkg["tester.remote"].Original.Ref}); err == nil {
		t.Fatal("partially installed batch accepted")
	}
	unchanged, _ := e.App.Loadout()
	if !unchanged.List.Equal(before) || e.App.Store.LastStepID(r.State.Plan.ID, "undo") != undoHead {
		t.Fatal("failed batch mutated list/history")
	}
	applied, err := e.App.ReplaceRecommended([]string{byPkg["tester.a"].Original.Ref, byPkg["tester.b"].Original.Ref})
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.State.List) != len(before)-1 || applied.State.List.Index(byPkg["tester.a"].Replacement.Instance) < 0 {
		t.Fatal("target not deduplicated", applied.State.List)
	}
	restored, _, err := e.App.UndoRedo(false)
	if err != nil || !restored.State.List.Equal(before) || e.App.Store.LastStepID(r.State.Plan.ID, "undo") != undoHead {
		t.Fatal("not one undo", err)
	}
	// The API payload must distinguish install and subscription choices.
	b, _ := json.Marshal(byPkg["tester.a"])
	t.Log(string(b))
}

func TestReplacementAlreadyEnabled(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Old"), testkit.Sample("tester.old", "Old"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "New"), testkit.Sample("tester.new", "New"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.old", "tester.new")
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[{"oldPackageId":"tester.old","newPackageId":"tester.new"}]}`)
	scanNow(t, e)
	r, err := e.App.Sort(false)
	if err != nil || len(r.Replacements) != 1 || !r.Replacements[0].Enabled {
		t.Fatal(r, err)
	}
	applied, err := e.App.ReplaceRecommended([]string{r.Replacements[0].Original.Ref})
	if err != nil || len(applied.State.List) != 2 {
		t.Fatal(applied, err)
	}
}

func TestMissingLineageRenameIsNotAnInstalledPackage(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	e.AddWorkshop("54321", testkit.Sample("tester.current", "Current name"))
	scanNow(t, e)
	current := one(t, e, "Current name")
	il := &app.ImportedList{Tokens: []string{"tester.old"}, Lineages: map[string]string{"tester.old": current.LineageID}}
	if _, err := e.App.ApplyImport(il, "renamed lineage", false); err != nil {
		t.Fatal(err)
	}
	r, err := e.App.Sort(false)
	if err != nil || len(r.MissingMods) != 1 {
		t.Fatal(r, err)
	}
	m := r.MissingMods[0]
	if m.Installed || m.CanEnable || m.Instance != "" || m.WorkshopID != "54321" {
		t.Fatal(m)
	}
}

func TestReplacementMustMatchRecommendedWorkshop(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Old"), testkit.Sample("tester.old", "Old"))
	e.AddWorkshop("7001", testkit.Sample("tester.new", "Different fork"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.old")
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[{"oldPackageId":"tester.old","newPackageId":"tester.new","newWorkshopId":"7002","newName":"Recommended fork"}]}`)
	scanNow(t, e)
	r, err := e.App.Sort(false)
	if err != nil || len(r.Replacements) != 1 {
		t.Fatal(r, err)
	}
	s := r.Replacements[0]
	if s.Installed || s.CanReplace || s.Replacement.WorkshopID != "7002" {
		t.Fatal(s)
	}
	if _, err := e.App.ReplaceRecommended([]string{s.Original.Ref}); err == nil {
		t.Fatal("wrong Workshop fork accepted")
	}
}
