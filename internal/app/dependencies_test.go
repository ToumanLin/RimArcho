package app_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestMissingDependenciesClassification(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	lib := testkit.Sample("tester.lib", "Library")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lib"), lib)
	// A rejected duplicate sorts before the usable copy by display name.
	e.WriteMod(filepath.Join(e.Game, "Mods", "ZDuplicate"), testkit.Sample("tester.lib", "AAA rejected"))
	e.WriteMod(filepath.Join(e.Game, "Data", "Biotech"), testkit.Sample("ludeon.rimworld.biotech", "Biotech"))
	about := `<ModMetaData><packageId>tester.a</packageId><name>Alpha</name>
 <modDependencies>
 <li><packageId> Tester.LIB_STEAM </packageId></li>
 <li><packageId>tester.lib</packageId></li>
 <li><packageId>tester.workshop</packageId><displayName>Workshop library</displayName><steamWorkshopUrl>steam://url/CommunityFilePage/123456</steamWorkshopUrl></li>
 <li><packageId>tester.unknown</packageId><displayName>Unknown library</displayName></li>
 <li><packageId>Ludeon.RimWorld</packageId></li>
 <li><packageId>Ludeon.RimWorld.Biotech</packageId></li>
 <li><packageId>Ludeon.RimWorld.Royalty</packageId><steamWorkshopUrl>https://steamcommunity.com/sharedfiles/filedetails/?id=999</steamWorkshopUrl></li>
 </modDependencies>
 <modDependenciesByVersion><v1.6><li><packageId>tester.current</packageId></li></v1.6><v1.7><li><packageId>tester.other</packageId></li></v1.7></modDependenciesByVersion>
 <loadAfter><li>tester.optional</li></loadAfter></ModMetaData>`
	e.WriteFile(filepath.Join(e.Game, "Mods", "A", "About", "About.xml"), about)
	b := testkit.Sample("tester.b", "Beta")
	b.Deps = []string{"TESTER.LIB", "tester.workshop", "tester.unknown"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), b)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a", "tester.b", "tester.unknown")
	scanNow(t, e)
	e.MustRun("alias", "pkg:tester.b", "Beta alias")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.App.MissingDependencies(st)
	if err != nil {
		t.Fatal(err)
	}
	byPkg := map[string]app.MissingDependency{}
	for _, d := range got {
		byPkg[d.PackageID] = d
	}
	if len(got) != 6 {
		t.Fatalf("dependencies: %+v", got)
	}
	libDep := byPkg["tester.lib"]
	if libDep.Kind != "installed" || libDep.Name != "Library" || libDep.Ref == "" || libDep.Instance == "" {
		t.Fatalf("library: %+v", libDep)
	}
	for _, pkg := range []string{"tester.lib", "tester.workshop", "tester.unknown"} {
		if !reflect.DeepEqual(byPkg[pkg].RequiredBy, []string{"Alpha", "Beta alias"}) {
			t.Fatalf("owners: %+v", byPkg[pkg])
		}
	}
	if d := byPkg["tester.workshop"]; d.Kind != "workshop" || d.WorkshopID != "123456" || d.Name != "Workshop library" {
		t.Fatalf("workshop: %+v", d)
	}
	for _, pkg := range []string{"tester.unknown", "tester.current", "ludeon.rimworld.royalty"} {
		if d := byPkg[pkg]; d.Kind != "unknown" || d.WorkshopID != "" {
			t.Fatalf("unknown: %+v", d)
		}
	}
	if byPkg["ludeon.rimworld.biotech"].Kind != "installed" {
		t.Fatal("installed DLC missing", got)
	}
	fs, err := e.App.DeclFindings(st)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if f.Code == "dep.inactive" && f.Key == "tester.lib" && f.Subjects[1] != "@{"+libDep.Instance+"}" {
			// Finding tokens use the same full instance chosen by the sort prompt.
			t.Fatalf("needs chose a different copy: %+v vs %+v", f, libDep)
		}
	}
	r, err := e.App.Sort(true)
	if err != nil || !reflect.DeepEqual(r.MissingDependencies, got) {
		t.Fatalf("dry sort: %+v %v", r, err)
	}
	e.MustRun("enable", libDep.Ref)
	st, _ = e.App.Loadout()
	got, err = e.App.MissingDependencies(st)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got {
		if d.PackageID == "tester.lib" {
			t.Fatal("enabled _steam dependency reported", d)
		}
	}
}

func TestCompleteDependenciesClosureAndUndo(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	for _, m := range []testkit.Mod{
		{PackageID: "tester.a", Name: "Alpha", Deps: []string{"tester.b", "tester.remote"}},
		{PackageID: "tester.b", Name: "Beta", Deps: []string{"tester.c"}},
		{PackageID: "tester.c", Name: "Gamma", Deps: []string{"tester.b", "tester.remote2"}},
	} {
		e.WriteMod(filepath.Join(e.Game, "Mods", m.Name), m)
	}
	// A cached steamDB index provides names and a shared Workshop ID.
	index, _ := json.Marshal(map[string][2]string{"tester.remote": {"123", "Remote"}, "tester.remote2": {"123", "Remote"}})
	e.WriteFile(filepath.Join(e.Home, "cache", "steamDB.index.json"), string(index))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a")
	scanNow(t, e)
	st, _ := e.App.Loadout()
	before := st.List.Clone()
	last := e.App.Store.LastStepID(st.Plan.ID, "undo")
	r, err := e.App.CompleteDependencies()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Enabled) != 2 || len(r.Edit.State.List) != 4 || !slices.Equal(r.WorkshopIDs, []string{"123"}) {
		t.Fatalf("completion: %+v", r)
	}
	names := active(t, e)
	if !slices.Equal(names, []string{"Core", "Beta", "Gamma", "Alpha"}) {
		t.Fatalf("unsorted: %v", names)
	}
	for _, d := range r.MissingDependencies {
		if d.Kind == "installed" {
			t.Fatal("still inactive", d)
		}
	}
	again, err := e.App.CompleteDependencies()
	if err != nil || len(again.Enabled) != 0 || len(again.Edit.Changes) != 0 {
		t.Fatalf("repeated completion: %+v %v", again, err)
	}
	undo, _, err := e.App.UndoRedo(false)
	if err != nil || !undo.State.List.Equal(before) {
		t.Fatalf("one undo did not restore original list: %+v %v", undo, err)
	}
	if e.App.Store.LastStepID(st.Plan.ID, "undo") != last {
		t.Fatal("completion left extra undo steps")
	}
}

func TestMissingDependenciesLocationAliasAndSteamCMD(t *testing.T) {
	t.Parallel()
	e := setup(t, true)
	for _, name := range []string{"Alias", "Deploy"} {
		e.WriteMod(filepath.Join(e.SteamCMD, name), testkit.Sample("tester."+name, name))
	}
	m := testkit.Sample("tester.needy", "Needy")
	m.Deps = []string{"tester.alias", "TESTER.DEPLOY_STEAM"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "Needy"), m)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.needy")
	scanNow(t, e)
	original := one(t, e, "Alias").ID
	if err := source.CreateLink(filepath.Join(e.Game, "Mods", "Aliased"), filepath.Join(e.SteamCMD, "Alias")); err != nil {
		t.Fatal(err)
	}
	scanNow(t, e)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Aliases[original]) == 0 {
		t.Fatal("fixture has no location alias")
	}
	deps, err := e.App.MissingDependencies(st)
	if err != nil || len(deps) != 2 {
		t.Fatalf("deps: %+v %v", deps, err)
	}
	for _, d := range deps {
		if d.Kind != "installed" {
			t.Fatal("SteamCMD/aliased dependency not enableable", d)
		}
		if d.PackageID == "tester.alias" && d.Instance != original {
			t.Fatal("alias created a second instance", d)
		}
	}
	r, err := e.App.CompleteDependencies()
	if err != nil || len(r.Enabled) != 2 || len(r.MissingDependencies) != 0 {
		t.Fatalf("complete alias/deployment: %+v %v", r, err)
	}
}
