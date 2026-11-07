package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// conseqLog is MCP eval h1 in small: a patch names a misspelled class in a def file, the
// file fails to load, and its defs' references and null-stat config checks follow.
const conseqLog = `Exception loading def from file Stats_Basics_General.xml: System.ArgumentException: Could not find type named Patcher.StatParts_Bin
Could not resolve cross-reference: No RimWorld.StatDef named Beauty found to give to Verse.ThingDef Bed_Fancy (using undefined instead)
Could not resolve cross-reference: No RimWorld.StatDef named Beauty found to give to Verse.ThingDef Bed_Plain (using undefined instead)
Could not resolve cross-reference to RimWorld.StatDef named BeautyOutdoors
Config error in Filth_Dirt: defines the stat base  more than once.
Could not resolve cross-reference: No RimWorld.StatDef named Unrelated found to give to Verse.ThingDef Bed_Fancy (using undefined instead)
`

func conseqEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	mods := filepath.Join(e.Game, "Mods")
	e.WriteMod(filepath.Join(mods, "Stats"), testkit.Mod{PackageID: "tester.stats", Name: "Stats", Author: "Tester", Files: map[string]string{
		"Defs/Stats/Stats_Basics_General.xml": "<Defs>\n  <StatDef>\n    <defName>Beauty</defName>\n  </StatDef>\n  <StatDef>\n    <defName>BeautyOutdoors</defName>\n  </StatDef>\n</Defs>\n",
	}})
	e.WriteMod(filepath.Join(mods, "Patcher"), testkit.Mod{PackageID: "tester.patcher", Name: "Patcher", Author: "Tester", Files: map[string]string{
		"Patches/Core.xml": "<Patch>\n  <Operation Class=\"PatchOperationAdd\">\n    <xpath>Defs/StatDef[defName=\"Beauty\"]</xpath>\n    <value>\n      <parts>\n        <li Class=\"Patcher.StatParts_Bin\"/>\n      </parts>\n    </value>\n  </Operation>\n</Patch>\n",
	}})
	e.WriteMod(filepath.Join(mods, "Bed"), testkit.Mod{PackageID: "tester.bed", Name: "Bed", Author: "Tester", Files: map[string]string{
		"Defs/Beds.xml": "<Defs>\n  <ThingDef>\n    <defName>Bed_Fancy</defName>\n    <statBases>\n      <Beauty>3</Beauty>\n    </statBases>\n  </ThingDef>\n</Defs>\n",
	}})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.stats", "tester.patcher", "tester.bed")
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	return e
}

// A def file that failed to load: the clusters that follow from its missing defs are its
// consequences, folded under it in the list, in test runs and in get_def (06 §3, MCP
// review round 2 P21); the patch naming the missing class is the root's source (P5).
func TestFailedDefFileConsequences(t *testing.T) {
	t.Parallel()
	e := conseqEnv(t)
	writePlayerLog(e, conseqLog)
	list := e.MustRun("log")
	for _, want := range []string{
		"log:E1 xml-load x1", "→ #", "(source", "+ 4 consequent lines in 4 clusters",
		"Unrelated", "folded: 4 clusters that follow from a def file that failed to load",
	} {
		if !strings.Contains(list, want) {
			t.Errorf("list misses %q:\n%s", want, list)
		}
	}
	for _, not := range []string{"named Beauty found", "BeautyOutdoors", "stat base"} {
		if strings.Contains(list, not) {
			t.Errorf("list shows the consequence %q:\n%s", not, list)
		}
	}
	all := e.MustRun("log", "--all")
	if !strings.Contains(all, "BeautyOutdoors") || strings.Count(all, "(consequence of log:E1)") != 4 {
		t.Errorf("--all:\n%s", all)
	}
	root := e.MustRun("log", "--cluster", "E1")
	for _, want := range []string{
		"names class Patcher.StatParts_Bin at Patches/Core.xml:6, which no installed mod defines",
		"consequences: 4 lines in 4 clusters", "StatDef/Beauty", "StatDef/BeautyOutdoors",
	} {
		if !strings.Contains(root, want) {
			t.Errorf("root cluster misses %q:\n%s", want, root)
		}
	}
	xref := e.MustRun("log", "--cluster", "E2")
	if !strings.Contains(xref, "consequence of log:E1: Stats_Basics_General.xml failed to load, so StatDef Beauty does not exist") {
		t.Errorf("consequence cluster:\n%s", xref)
	}
	def := e.MustRun("def", "StatDef/Beauty")
	if !strings.Contains(def, "not loaded: the newest log says Stats_Basics_General.xml failed to load (log:E1)") {
		t.Errorf("def:\n%s", def)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	r := testRunWith(t, e, "menu", conseqLog)
	if r.ClustersTotal != 6 || len(r.Clusters) != 2 || r.Clusters[0].Consequences != 4 || r.Clusters[1].Consequences != 0 {
		t.Fatalf("test run clusters %d %+v", r.ClustersTotal, r.Clusters)
	}
}
