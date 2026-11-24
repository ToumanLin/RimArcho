package app_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 08 §1.6: two Mods replacing the same node are shown, never sorted for: the sort preview
// names a winner the sort changes and the rule that keeps it, the problem says who wins
// now, and `rule add ignore contested:…` hides the pair. A Mod that only patches another's
// Def names nothing. Only the stored analysis is read.
func TestSamePatchTargetIsInformation(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	bare := func(pkg, name string) testkit.Mod {
		return testkit.Mod{PackageID: pkg, Name: name, Author: "Tester", Files: map[string]string{}}
	}
	replace := func(v string) string {
		return `<Patch><Operation Class="PatchOperationReplace"><xpath>Defs/ThingDef[defName="Lib_Gun"]/statBases/MarketValue</xpath><value><MarketValue>` + v + `</MarketValue></value></Operation></Patch>`
	}
	lib := bare("tester.lib", "Lib")
	lib.Files["Defs/Lib.xml"] = `<Defs><ThingDef><defName>Lib_Gun</defName><statBases><MarketValue>10</MarketValue></statBases></ThingDef></Defs>`
	alpha := bare("tester.alpha", "Alpha")
	alpha.Files["Patches/A.xml"] = replace("20")
	beta := bare("tester.beta", "Beta")
	beta.Files["Patches/B.xml"] = replace("30")
	// Beta asks to load before Alpha: the sort moves it, and Alpha's value wins after
	beta.Files["About/About.xml"] = strings.Replace(beta.AboutXML(), "</ModMetaData>", "  <loadBefore><li>tester.alpha</li></loadBefore>\n</ModMetaData>", 1)
	adder := bare("tester.adder", "Adder")
	adder.Files["Patches/C.xml"] = `<Patch><Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Lib_Gun"]</xpath><value><x/></value></Operation></Patch>`
	for _, m := range []testkit.Mod{lib, alpha, beta, adder} {
		e.WriteMod(filepath.Join(e.Game, "Mods", m.Name), m)
	}
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.adder", "tester.lib", "tester.alpha", "tester.beta")
	scanNow(t, e)

	if out := e.MustRun("sort", "--dry-run"); !strings.Contains(out, "same patch target: "+app.ContestNoAnalysis) {
		t.Fatalf("before the analysis:\n%s", out)
	}
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	out := e.MustRun("sort", "--dry-run")
	// only the declared loadBefore moves anything
	if !strings.Contains(out, "sort: 1 moved") {
		t.Fatalf("moves:\n%s", out)
	}
	st := mustLoadout(t, e.App)
	ref := func(pkg string) string {
		for _, m := range st.Ix.Mods {
			if m.PackageID == pkg {
				return st.Ix.Ref(m.ID)
			}
		}
		t.Fatalf("no %s", pkg)
		return ""
	}
	a, b := ref("tester.alpha"), ref("tester.beta")
	want := "ThingDef Lib_Gun's statBases/MarketValue: " + b + " wins now, " + a + " after this sort  (keep " + b + ": archo rule add after " + b[1:] + " " + a[1:] + ")"
	if !strings.Contains(out, want) {
		t.Fatalf("no winner change line:\n%s", out)
	}
	if strings.Count(out, "Lib_Gun") != 1 {
		t.Fatalf("Adder's Add named a contest:\n%s", out)
	}
	pr := e.MustRun("problems", "--code", "patch.contested")
	if !strings.Contains(pr, "both change ThingDef Lib_Gun's statBases/MarketValue; the one loaded later wins: currently "+b) ||
		!strings.Contains(pr, "archo rule add ignore contested:") {
		t.Fatalf("problems:\n%s", pr)
	}
	// the printed reference hides the pair in both places
	cref := regexp.MustCompile(`contested:#\w+:#\w+`).FindString(pr)
	e.MustRun("rule", "add", "ignore", cref)
	if out := e.MustRun("sort", "--dry-run"); strings.Contains(out, "same patch target") {
		t.Fatalf("after ignoring %s:\n%s", cref, out)
	}
	if pr := e.MustRun("problems", "--code", "patch.contested"); strings.Contains(pr, "Lib_Gun") {
		t.Fatalf("problems after ignoring %s:\n%s", cref, pr)
	}
	if rl := e.MustRun("rule", "list"); !strings.Contains(rl, cref) {
		t.Fatalf("rule list:\n%s", rl)
	}
}
