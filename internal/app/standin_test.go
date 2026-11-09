package app_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// translationAbout requires tester.old; alts become its alternativePackageIds.
func translationAbout(alts ...string) string {
	li := ""
	if len(alts) > 0 {
		li = "<alternativePackageIds><li>" + strings.Join(alts, "</li><li>") + "</li></alternativePackageIds>"
	}
	return `<ModMetaData><packageId>tester.translation</packageId><name>Translation</name>
 <supportedVersions><li>1.6</li></supportedVersions>
 <modDependencies><li><packageId>tester.old</packageId><displayName>Old</displayName>` + li + `</li></modDependencies></ModMetaData>`
}

// standInSetup: Old (1.4 only, installed, not enabled), New (a continuation with another
// packageId, enabled) and Translation, which requires Old and comes before New in the list.
func standInSetup(t *testing.T, alts ...string) *testkit.Env {
	e := setup(t, false)
	old := testkit.Sample("tester.old", "Old")
	old.Supported = []string{"1.4"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "Old"), old)
	e.WriteMod(filepath.Join(e.Game, "Mods", "New"), testkit.Sample("tester.new", "New"))
	e.WriteFile(filepath.Join(e.Game, "Mods", "Translation", "About", "About.xml"), translationAbout(alts...))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.translation", "tester.new")
	return e
}

func depCodes(t *testing.T, e *testkit.Env) []string {
	t.Helper()
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range ps.List {
		if strings.HasPrefix(p.Code, "dep.") {
			out = append(out, p.Code+" "+p.Key)
		}
	}
	return out
}

// checkStandIn: the dependency counts as met (no finding, nothing for the sort to offer),
// the sort puts Translation after New, and the details show New in place of Old.
func checkStandIn(t *testing.T, e *testkit.Env, how string) {
	t.Helper()
	if c := depCodes(t, e); len(c) > 0 {
		t.Fatalf("dependency findings: %v", c)
	}
	r, err := e.App.Sort(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.MissingDependencies) != 0 {
		t.Fatalf("missing: %+v", r.MissingDependencies)
	}
	if got := names(t, e); got != "Core New Translation" {
		t.Fatal(got)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	m, kind := e.App.DependencyStandIn(st, scan.Dependency{PackageID: "tester.old", Alternatives: altsOf(t, e)})
	if m == nil || m.Name != "New" || kind != how {
		t.Fatalf("stand-in %v %q", m, kind)
	}
}

func altsOf(t *testing.T, e *testkit.Env) []string {
	t.Helper()
	ms := named(t, e, "Translation")
	d := ms[0].Decl()
	return d.Dependencies[0].Alternatives
}

// The game's ModDependency.IsSatisfied accepts an enabled alternativePackageIds entry.
func TestDependencyAlternativeMeets(t *testing.T) {
	t.Parallel()
	e := standInSetup(t, "Tester.Other", "tester.new")
	scanNow(t, e)
	checkStandIn(t, e, app.StandInAlternative)
}

// Use This Instead names New as Old's replacement: the translation of Old is met by New.
func TestDependencyReplacementMeets(t *testing.T) {
	t.Parallel()
	e := standInSetup(t)
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[{"oldPackageId":"tester.old","oldWorkshopId":"1001","newPackageId":"tester.new","newWorkshopId":"1002","newName":"New","newVersions":["1.6"]}]}`)
	scanNow(t, e)
	checkStandIn(t, e, app.StandInReplacement)
	// Old itself, enabled again, is what the dependency names: no stand-in then.
	e.MustRun("enable", "pkg:tester.old")
	st, _ := e.App.Loadout()
	if m, _ := e.App.DependencyStandIn(st, scan.Dependency{PackageID: "tester.old"}); m != nil {
		t.Fatal("stand-in next to the package itself", m.Name)
	}
}

// Without an alternative or a replacement the dependency is still missing.
func TestDependencyNoStandIn(t *testing.T) {
	t.Parallel()
	e := standInSetup(t)
	scanNow(t, e)
	if c := depCodes(t, e); len(c) != 1 || c[0] != "dep.inactive tester.old" {
		t.Fatalf("dependency findings: %v", c)
	}
}

// A scan after an upgrade brings stored declarations of unchanged mods up to date
// (alternativePackageIds were not read before), without recording an update.
func TestDeclRefreshedAfterParserUpgrade(t *testing.T) {
	t.Parallel()
	e := standInSetup(t, "tester.new")
	scanNow(t, e)
	tr := named(t, e, "Translation")[0]
	var d scan.Decl
	var blob string
	if err := e.App.Store.DB.QueryRow(`SELECT decl_blob FROM snapshot WHERE id=?`, tr.SnapID).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal([]byte(blob), &d)
	d.Dependencies[0].Alternatives = nil // as the previous parser stored it
	b, _ := json.Marshal(d)
	if _, err := e.App.Store.DB.Exec(`UPDATE snapshot SET decl_blob=? WHERE id=?`, string(b), tr.SnapID); err != nil {
		t.Fatal(err)
	}
	if c := depCodes(t, e); len(c) != 1 {
		t.Fatalf("old declaration: %v", c)
	}
	var snaps int
	e.App.Store.DB.QueryRow(`SELECT count(*) FROM snapshot WHERE instance_id=?`, tr.ID).Scan(&snaps)
	scanNow(t, e)
	after := named(t, e, "Translation")[0]
	if after.SnapID != tr.SnapID {
		t.Fatal("a new snapshot for unchanged files")
	}
	var n int
	e.App.Store.DB.QueryRow(`SELECT count(*) FROM snapshot WHERE instance_id=?`, tr.ID).Scan(&n)
	if n != snaps {
		t.Fatal("snapshots", snaps, n)
	}
	if got := after.Decl().Dependencies[0].Alternatives; len(got) != 1 || got[0] != "tester.new" {
		t.Fatal(got)
	}
	if c := depCodes(t, e); len(c) != 0 {
		t.Fatalf("after rescan: %v", c)
	}
}

// The player's replaces rule: New stands in for Old (given as pkg: or as the installed mod).
func TestDependencyRuleMeets(t *testing.T) {
	t.Parallel()
	for _, b := range []string{"pkg:Tester.Old", "Old"} {
		e := standInSetup(t)
		scanNow(t, e)
		if b == "Old" {
			ix, err := e.App.Index()
			if err != nil {
				t.Fatal(err)
			}
			b = ix.Ref(named(t, e, "Old")[0].ID)
		}
		e.MustRun("rule", "add", "replaces", "pkg:tester.new", b, "--note", "continued")
		checkStandIn(t, e, app.StandInRule)
	}
	e := standInSetup(t)
	scanNow(t, e)
	if _, code := e.Run("rule", "add", "replaces", "pkg:tester.new", "pkg:tester.new"); code == 0 {
		t.Fatal("a mod replacing itself")
	}
	if _, code := e.Run("rule", "add", "replaces", "pkg:tester.new"); code == 0 {
		t.Fatal("replaces with one reference")
	}
}

// A dependency the player ignored is not offered after a sort, nor enabled by completing
// dependencies; the ignore lapses when the requiring mod changes.
func TestIgnoredDependencyNotOffered(t *testing.T) {
	t.Parallel()
	e := standInSetup(t)
	scanNow(t, e)
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := ""
	for _, p := range ps.List {
		if p.Code == "dep.inactive" {
			ref = p.Ref
		}
	}
	if ref == "" {
		t.Fatal("no dep.inactive")
	}
	e.MustRun("problems", "ignore", ref)
	r, err := e.App.Sort(false)
	if err != nil || len(r.MissingDependencies) != 0 {
		t.Fatal(r.MissingDependencies, err)
	}
	c, err := e.App.CompleteDependencies()
	if err != nil || len(c.Enabled) != 0 {
		t.Fatal(c, err)
	}
	// The translation changes: asked again.
	e.WriteFile(filepath.Join(e.Game, "Mods", "Translation", "About", "About.xml"), strings.Replace(translationAbout(), "<name>Translation</name>", "<name>Translation</name><description>2</description>", 1))
	scanNow(t, e)
	if r, err = e.App.Sort(false); err != nil || len(r.MissingDependencies) != 1 {
		t.Fatal(r.MissingDependencies, err)
	}
}
