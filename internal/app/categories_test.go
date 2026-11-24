package app_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// TestModCategories: a Mod's category comes from what is inside it, never its name (16 §2.5);
// the player's category replaces it, filters use it, and clearing gives the automatic one back.
func TestModCategories(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	mods := e.Game + "/Mods/"
	only := func(pkg, name string, files map[string]string, deps ...string) testkit.Mod {
		return testkit.Mod{PackageID: pkg, Name: name, Author: "Tester", Deps: deps, Files: files}
	}
	lib := testkit.Sample("tester.lib", "Shared Framework")
	lib.Files["Assemblies/Framework.dll"] = "MZ"
	e.WriteMod(mods+"Lib", lib)
	for _, n := range []string{"A", "B", "C", "D"} {
		m := testkit.Sample("tester.user"+strings.ToLower(n), "User "+n)
		m.Deps = []string{"tester.lib"}
		e.WriteMod(mods+"User"+n, m)
	}
	// required by fewer than LibraryMinDependents Mods: still content
	small := testkit.Sample("tester.small", "Small Framework")
	small.Files["Assemblies/Small.dll"] = "MZ"
	e.WriteMod(mods+"Small", small)
	e.WriteMod(mods+"SmallUser", func() testkit.Mod {
		m := testkit.Sample("tester.smalluser", "Small User")
		m.Deps = []string{"tester.small"}
		return m
	}())
	e.WriteMod(mods+"Patch", only("tester.patch", "Patch Only", map[string]string{
		"1.6/Patches/a.xml": "<Patch/>", "Patches/b.xml": "<Patch/>", "Languages/English/Keyed/x.xml": "<LanguageData/>"}))
	e.WriteMod(mods+"Code", only("tester.code", "Code Only", map[string]string{"1.6/Assemblies/Code.dll": "MZ", "Patches/c.xml": "<Patch/>"}))
	e.WriteMod(mods+"Lang", only("tester.lang", "Chinese Pack", map[string]string{
		"Languages/ChineseSimplified/Keyed/a.xml": "<LanguageData/>", "Textures/UI/a.png": "x"}))
	e.WriteMod(mods+"Tex", only("tester.tex", "Retexture", map[string]string{"Textures/Things/a.png": "x"}))
	// the name says patch and translation; the content says content
	e.WriteMod(mods+"Misnamed", testkit.Sample("tester.misnamed", "Translation Patch"))
	e.WriteMod(mods+"Empty", only("tester.empty", "Sounds Only", map[string]string{"Sounds/a.ogg": "x"}))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.lib", "tester.patch")
	scanNow(t, e)

	want := map[string]string{
		"ludeon.rimworld": "game", "tester.lib": "library", "tester.usera": "content", "tester.small": "content",
		"tester.patch": "patches", "tester.code": "code", "tester.lang": "translation", "tester.tex": "textures",
		"tester.misnamed": "content", "tester.empty": "",
	}
	cats := func() map[string]app.Category {
		ix, err := e.App.Index()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]app.Category{}
		for _, m := range ix.Mods {
			out[m.PackageID] = ix.CategoryFor(m.ID)
		}
		return out
	}
	got := cats()
	for pkg, k := range want {
		if got[pkg].Key != k || got[pkg].Player {
			t.Errorf("%s: %+v, want %q", pkg, got[pkg], k)
		}
	}
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	detail := func(pkg string) app.Category {
		for _, m := range ix.Mods {
			if m.PackageID == pkg {
				return ix.CategoryDetail(m.ID)
			}
		}
		return app.Category{}
	}
	if w := detail("tester.lib").Why; w.Dependents != 4 || w.DLLs != 1 || w.Defs != 2 {
		t.Errorf("library facts: %+v", w)
	}
	if w := detail("tester.patch").Why; w.Patches != 2 || w.Defs != 0 || !w.Languages {
		t.Errorf("patch facts: %+v", w)
	}
	if r := app.CategoryReason(detail("tester.patch")); r != "only patches: 2 XML in Patches, no Defs or DLL" {
		t.Errorf("reason: %q", r)
	}
	if r := app.CategoryReason(detail("tester.lib")); r != "4 installed Mods require it, and it ships a DLL" {
		t.Errorf("reason: %q", r)
	}

	// The player's category, from the CLI; mod details show it with the automatic one.
	if out := e.MustRun("category", "pkg:tester.code", "library"); !strings.Contains(out, "library (set by you; automatic: code)") {
		t.Fatalf("set: %s", out)
	}
	if c := cats()["tester.code"]; c.Key != "library" || c.Auto != "code" || !c.Player {
		t.Fatalf("player category: %+v", c)
	}
	if out := e.MustRun("mod", "pkg:tester.patch"); !strings.Contains(out, "patches (only patches: 2 XML in Patches, no Defs or DLL)") {
		t.Fatalf("mod details: %s", out)
	}
	if _, code := e.Run("category", "pkg:tester.code", "interface"); code == 0 {
		t.Fatal("an unknown category was taken")
	}

	// The editor's rows carry the category; the bar and the search filter by it.
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	names := func(f *view.BarFilter) []string {
		var out []string
		for col, rows := range f.Rows {
			for _, i := range rows {
				out = append(out, b.Cols[col][i].Package)
			}
		}
		return out
	}
	if got := names(view.FilterBoard(b, "", []string{"category:library"}, view.MatchFacts{}, false, "1.6")); strings.Join(got, ",") != "tester.lib,tester.code" && strings.Join(got, ",") != "tester.code,tester.lib" {
		t.Fatalf("category:library rows: %v", got)
	}
	if got := names(view.FilterBoard(b, "category:patches", nil, view.MatchFacts{}, false, "1.6")); strings.Join(got, ",") != "tester.patch" {
		t.Fatalf("category:patches search: %v", got)
	}
	f := view.FilterBoard(b, "", nil, view.MatchFacts{}, true, "1.6")
	found := false
	for _, g := range f.Groups {
		if g.Key == "category" {
			found = len(g.Values) > 0
		}
	}
	if !found {
		t.Fatal("no category group in the bar")
	}

	// Clearing gives the automatic one back.
	e.MustRun("category", "pkg:tester.code", "--clear")
	if c := cats()["tester.code"]; c.Key != "code" || c.Player {
		t.Fatalf("cleared: %+v", c)
	}
}
