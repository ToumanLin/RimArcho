package app_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 08 §1.6: a Mod whose XML patches change Defs only one other enabled Mod defines sorts
// after it, from the stored analysis only; a Def two enabled Mods define names nobody;
// `rule add ignore` takes the printed reference.
func TestSortInferredFromPatches(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	bare := func(pkg, name string) testkit.Mod {
		return testkit.Mod{PackageID: pkg, Name: name, Author: "Tester", Files: map[string]string{}}
	}
	lib := bare("tester.lib", "Lib")
	lib.Files["Defs/Lib.xml"] = `<Defs><ThingDef><defName>Lib_Gun</defName></ThingDef><HediffDef><defName>Lib_Wound</defName></HediffDef><ThingDef><defName>Shared_Def</defName></ThingDef></Defs>`
	other := bare("tester.other", "Other")
	other.Files["Defs/Other.xml"] = `<Defs><ThingDef><defName>Shared_Def</defName></ThingDef></Defs>`
	patcher := bare("tester.patcher", "Patcher")
	patcher.Files["Patches/P.xml"] = `<Patch>
  <Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Lib_Gun"]</xpath><value><a/></value></Operation>
  <Operation Class="PatchOperationFindMod">
    <mods><li>Lib</li></mods>
    <match Class="PatchOperationAdd"><xpath>Defs/HediffDef[defName="Lib_Wound"]</xpath><value><b/></value></match>
  </Operation>
</Patch>`
	amb := bare("tester.amb", "Amb")
	amb.Files["Patches/A.xml"] = `<Patch><Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Shared_Def"]</xpath><value><c/></value></Operation></Patch>`
	for _, m := range []testkit.Mod{lib, other, patcher, amb} {
		e.WriteMod(filepath.Join(e.Game, "Mods", m.Name), m)
	}
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.patcher", "tester.amb", "tester.lib", "tester.other")
	scanNow(t, e)

	// no stored analysis: sorted without inferred rules, and said so
	out := e.MustRun("sort", "--dry-run")
	if !strings.Contains(out, "sort: 0 moved") || !strings.Contains(out, app.InferredNoAnalysis) {
		t.Fatalf("before the analysis:\n%s", out)
	}
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	out = e.MustRun("sort", "--dry-run")
	if !strings.Contains(out, "sort: 1 moved") || !strings.Contains(out, "patches its HediffDefs, ThingDefs (2 patches)") ||
		!strings.Contains(out, "inferred: 1 preference from XML patches") {
		t.Fatalf("after the analysis:\n%s", out)
	}
	if strings.Contains(out, "Amb") && strings.Contains(out, "inferred #") && strings.Count(out, "inferred #") > 1 {
		t.Fatalf("the ambiguous Def named a Mod:\n%s", out)
	}
	st := mustLoadout(t, e.App)
	rules, note, err := e.App.InferredRules(st, st.List)
	if err != nil || note != "" || len(rules) != 1 || st.Ix.ByID[rules[0].Patcher].PackageID != "tester.patcher" ||
		st.Ix.ByID[rules[0].Owner].PackageID != "tester.lib" || rules[0].Patches != 2 {
		t.Fatalf("rules %+v note %q err %v", rules, note, err)
	}

	// the printed reference works unchanged in rule add ignore
	ref := regexp.MustCompile(`inferred:#\w+:#\w+`).FindString(e.MustRun("sort", "--dry-run"))
	if ref == "" {
		t.Fatal("no printed inferred reference")
	}
	e.MustRun("rule", "add", "ignore", ref)
	if out := e.MustRun("sort", "--dry-run"); !strings.Contains(out, "sort: 0 moved") || !strings.Contains(out, "1 ignored by rules") {
		t.Fatalf("after ignoring %s:\n%s", ref, out)
	}
	if out := e.MustRun("rule", "list"); !strings.Contains(out, ref) {
		t.Fatalf("rule list does not print %s:\n%s", ref, out)
	}
}
