package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 05 §4.4 "Between checks": after a list edit, a patch whose target Mod is no longer
// enabled shows at once from the last check's facts, and one whose target was enabled
// goes away at once; a guarded patch (FindMod) never does; the next check agrees.
func TestPatchTargetsFollowTheListBetweenChecks(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	lib := testkit.Sample("tester.lib", "Lib")
	lib.Files["Defs/Lib.xml"] = "<Defs><ThingDef><defName>Lib_Gun</defName></ThingDef><ThingDef><defName>Lib_Axe</defName></ThingDef></Defs>\n"
	delete(lib.Files, "Patches/lib.xml")
	open := testkit.Sample("tester.open", "Open")
	open.Files["Patches/Open.xml"] = `<Patch>
  <Operation Class="PatchOperationAdd">
    <xpath>Defs/ThingDef[defName="Lib_Gun"]</xpath>
    <value><description>top</description></value>
  </Operation>
</Patch>
`
	delete(open.Files, "Patches/open.xml")
	guarded := testkit.Sample("tester.guarded", "Guarded")
	guarded.Files["Patches/Guarded.xml"] = `<Patch>
  <Operation Class="PatchOperationFindMod">
    <mods><li>Lib</li></mods>
    <match Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Lib_Axe"]</xpath><value><x/></value></match>
  </Operation>
</Patch>
`
	delete(guarded.Files, "Patches/guarded.xml")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lib"), lib)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Open"), open)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Guarded"), guarded)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.lib", "tester.open", "tester.guarded")
	scanNow(t, e)
	noMatch := func() (quick, stored []string) {
		t.Helper()
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range ps.List {
			if p.Code == "patch.no-match" {
				if strings.HasPrefix(p.Key, "quick|") {
					quick = append(quick, p.Claim)
				} else {
					stored = append(stored, p.Claim)
				}
			}
		}
		return
	}
	check := func() {
		t.Helper()
		if _, err := e.App.EnsureAnalysis(nil); err != nil {
			t.Fatal(err)
		}
	}
	check()
	if q, s := noMatch(); len(q)+len(s) != 0 {
		t.Fatalf("with Lib enabled: quick %v stored %v", q, s)
	}
	before, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	// disable Lib: Open's unguarded patch shows at once; Guarded's FindMod patch does not
	e.MustRun("disable", "pkg:tester.lib")
	// the editor's incremental read (16 §8.7) gives the same
	inc, err := e.App.IncrementalProblems(before)
	if err != nil {
		t.Fatal(err)
	}
	var incQuick []string
	for _, p := range inc.List {
		if p.Code == "patch.no-match" {
			incQuick = append(incQuick, p.Key)
		}
	}
	if len(incQuick) != 1 || !strings.HasPrefix(incQuick[0], "quick|") {
		t.Fatalf("incremental after disabling Lib: %v", incQuick)
	}
	if a, _ := e.App.Analysis(mustLoadout(t, e.App)); a.Fresh {
		t.Fatal("the check should be stale after the edit")
	}
	q, s := noMatch()
	if len(q) != 1 || !strings.Contains(q[0], "Open.xml:2") || !strings.Contains(q[0], "target-inactive") || len(s) != 0 {
		t.Fatalf("after disabling Lib: quick %v stored %v", q, s)
	}
	// the check agrees, with its own finding
	check()
	q, s = noMatch()
	if len(q) != 0 || len(s) != 1 || !strings.Contains(s[0], "Open.xml:2") {
		t.Fatalf("after the check: quick %v stored %v", q, s)
	}
	// enable Lib again: the stored finding goes at once, before the next check
	e.MustRun("enable", "pkg:tester.lib", "--at", "2")
	if q, s := noMatch(); len(q)+len(s) != 0 {
		t.Fatalf("after enabling Lib: quick %v stored %v", q, s)
	}
}

func mustLoadout(t *testing.T, a *app.App) *app.LoadoutState {
	t.Helper()
	st, err := a.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	return st
}
