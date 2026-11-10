package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// listModLog: MCP eval h7 in synthetic form. A profiler's prefix replaces ThingWithComps.Tick
// with a foreach over AllComps and throws "Collection was modified"; the comp that changed the
// list (CompRemover.CompSelfRemover.CompTick) is not in the stack; Mod A only has a prefix on
// Pawn.Tick, a caller. The second entry has a patch whose TargetMethod() is computed.
const listModLog = `RimWorld 1.6.4633 rev1261
Exception ticking Popski (at (138, 0, 120)): System.InvalidOperationException: Collection was modified; enumeration operation may not execute.
[Ref 9D9F5F06]
 [0x00000] in <51fded79cd284d4d911c5949aff4cb21>:0
  at System.Collections.Generic.List` + "`" + `1+Enumerator[T].MoveNextRare () [0x00013] in <51fded79cd284d4d911c5949aff4cb21>:0
  at System.Collections.Generic.List` + "`" + `1+Enumerator[T].MoveNext () [0x0004a] in <51fded79cd284d4d911c5949aff4cb21>:0
  at CompThrower.Tick_Patch.Prefix (Verse.ThingWithComps __instance) [0x0004a] in <084a0936e2de4849a412b96097b60304>:0
  at Verse.ThingWithComps.Tick () [0x00008] in <61e4173561894da49d210260257b5097>:0
    - PREFIX tester.thrower: Boolean CompThrower.Tick_Patch:Prefix(ThingWithComps __instance)
  at Verse.Pawn.Tick () [0x00042] in <61e4173561894da49d210260257b5097>:0
    - PREFIX tester.moda.harmony: Boolean ModA.Pawn_Tick:Prefix(Pawn __instance)
  at Verse.Thing.DoTick () [0x0007b] in <61e4173561894da49d210260257b5097>:0
  at Verse.TickList.Tick () [0x00139] in <61e4173561894da49d210260257b5097>:0

Exception in Destroy: System.NullReferenceException: Object reference not set to an instance of an object
  at (wrapper dynamic-method) Verse.Thing.Verse.Thing.Destroy_Patch1(Verse.Thing)
    - PREFIX tester.thrower: Void CompThrower.Dyn_Computed:Prefix()
  at Verse.TickList.Tick () [0x00139] in <61e4173561894da49d210260257b5097>:0
`

// listModFixture:
//
//	Thrower  tester.thrower  ModCompThrower.dll: the foreach prefix, TargetMethod() patches
//	Remover  tester.remover  ModCompRemover.dll: CompSelfRemover (removes itself in CompTick),
//	                         Gizmo.Attach (AllComps.Add in UI code); Patches.xml adds the comp
//	                         to ThingDef Human; Defs name it through CompProperties_Remover
//	Race     tester.race     defines ThingDef Human
//	ModA     tester.moda     a bool prefix on Pawn.Tick (Harmony ID tester.moda.harmony)
func listModFixture(t *testing.T) (*testkit.Env, *app.LogReport) {
	e := testkit.New(t)
	mods := filepath.Join(e.Game, "Mods")
	mod := func(dir, pkg string, files map[string]string, dlls ...string) {
		e.WriteMod(filepath.Join(mods, dir), testkit.Mod{PackageID: pkg, Name: dir, Author: "Tester", Files: files})
		for _, d := range dlls {
			b, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), d))
			if err != nil {
				t.Fatal(err)
			}
			e.WriteFile(filepath.Join(mods, dir, "Assemblies", d), string(b))
		}
	}
	mod("Harmony", "brrainz.harmony", nil, "0Harmony.dll")
	mod("Thrower", "tester.thrower", nil, "ModCompThrower.dll")
	mod("Remover", "tester.remover", map[string]string{
		"Patches/Patches.xml": "<Patch>\n  <Operation Class=\"PatchOperationAdd\">\n    <xpath>Defs/ThingDef[defName=\"Human\"]/comps</xpath>\n    <value>\n      <li>\n        <compClass>CompRemover.CompSelfRemover</compClass>\n      </li>\n    </value>\n  </Operation>\n</Patch>\n",
		"Defs/Things.xml":     "<Defs>\n  <ThingDef>\n    <defName>Remover_Statue</defName>\n    <comps>\n      <li Class=\"CompRemover.CompProperties_Remover\" />\n    </comps>\n  </ThingDef>\n</Defs>\n",
	}, "ModCompRemover.dll")
	mod("Race", "tester.race", map[string]string{"Defs/Race.xml": "<Defs>\n  <ThingDef>\n    <defName>Human</defName>\n    <comps />\n  </ThingDef>\n</Defs>\n"})
	mod("ModA", "tester.moda", nil, "ModA.dll")
	e.WriteModsConfig("1.6.4633 rev1261", "brrainz.harmony", "ludeon.rimworld", "tester.race", "tester.thrower", "tester.remover", "tester.moda")
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	rep, err := e.App.AnalyzeLogText(listModLog)
	if err != nil {
		t.Fatal(err)
	}
	return e, rep
}

func TestListModifiers(t *testing.T) {
	e, rep := listModFixture(t)
	roles, text := ruleCluster(t, e, rep, "Collection was modified")
	want := map[string]string{"tester.thrower": "source", "tester.remover": "list-modifier", "tester.moda": "listed"}
	for k, v := range want {
		if roles[k] != v {
			t.Errorf("%s = %q, want %q (all %v)", k, roles[k], v, roles)
		}
	}
	for _, s := range []string{"CompRemover.CompSelfRemover.CompTick (GenList.Swap, AllComps.Remove)", "ThingComp.CompTick override",
		"CompRemover.Gizmo.Attach (AllComps.Add) (not in a tick method)", "a patch on a caller of the throwing frame"} {
		if !strings.Contains(text, s) {
			t.Errorf("evidence lacks %q:\n%s", s, text)
		}
	}
	var lc *app.LogCluster
	for _, c := range rep.Clusters {
		if strings.Contains(c.Key, "Collection was modified") {
			lc = c
		}
	}
	ls := lc.ListScan
	if ls == nil || ls.Enumerated != "Verse.ThingWithComps.AllComps" || ls.Mods != 3 || len(ls.Modifiers) != 2 {
		t.Fatalf("ListScan %+v", ls)
	}
	tick := ls.Modifiers[0]
	if tick.Tick != "CompTick" || len(tick.Attached) != 2 ||
		!strings.HasPrefix(tick.Attached[0], "ThingDef/Remover_Statue (as CompRemover.CompProperties_Remover) by ") ||
		!strings.HasPrefix(tick.Attached[1], "ThingDef/Human by ") || !strings.HasSuffix(tick.Attached[1], " Patches/Patches.xml:6") {
		t.Errorf("tick modifier %+v", tick)
	}
	if !slices.ContainsFunc(lc.Next, func(n string) bool { return strings.HasSuffix(n, "--type CompRemover.CompSelfRemover") }) {
		t.Errorf("next %q", lc.Next)
	}
	// The source search covers the modifier, not the caller's patcher.
	st, _ := e.App.Loadout()
	imp := e.App.ImplicatedMods(st)
	_ = imp // the text log is not the problem log; checked through the finding instead
	for _, s := range lc.Finding.Subjects {
		if strings.Contains(s, st.Ix.Ref(byPkg(t, st, "tester.moda"))) {
			t.Errorf("listed mod is a subject: %v", lc.Finding.Subjects)
		}
	}
	// A second scan reads the cache (per DLL hash).
	es, _ := os.ReadDir(filepath.Join(e.App.Home, "cache", "ilscan"))
	if len(es) == 0 {
		t.Error("no ilscan cache files")
	}
}

// A patch class whose TargetMethod() the analysis could not read: no `harmony --target` for its
// frame (it finds no row), the owner's patch class instead (MCP eval h4).
func TestDynamicPatchNext(t *testing.T) {
	_, rep := listModFixture(t)
	for _, lc := range rep.Clusters {
		if !strings.Contains(lc.Key, "Destroy") {
			continue
		}
		joined := strings.Join(lc.Next, "\n")
		if strings.Contains(joined, "archo harmony --target Verse.Thing.Destroy") || !strings.Contains(joined, "archo harmony --mod ") ||
			!strings.Contains(joined, "--type CompThrower.Dyn_Computed") {
			t.Errorf("next %q", lc.Next)
		}
		return
	}
	t.Fatal("no Destroy cluster")
}

func byPkg(t *testing.T, st *app.LoadoutState, pkg string) string {
	t.Helper()
	for _, m := range st.Ix.Mods {
		if strings.EqualFold(m.PackageID, pkg) {
			return m.ID
		}
	}
	t.Fatalf("no mod %s", pkg)
	return ""
}

// The CLI shows the scan and the enumerated list; `harmony --target` of a frame that only a
// dynamic patch is on lists it from the log; the default source search covers the modifier.
func TestListModCLI(t *testing.T) {
	e, _ := listModFixture(t)
	p := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(p, listModLog)
	e.RecordLaunch(e.Now().Add(-30 * time.Minute))
	os.Chtimes(p, e.Now(), e.Now())
	out := e.MustRun("log", "--cluster", "E1")
	for _, s := range []string{"enumerated: ThingWithComps.AllComps (foreach in ", "comp list changed by (candidates; 3 enabled mods, 3 assemblies scanned):",
		"CompRemover.CompSelfRemover.CompTick  GenList.Swap, AllComps.Remove  ← overrides ThingComp.CompTick", "attached to: ThingDef/Human by ",
		"CompRemover.Gizmo.Attach  AllComps.Add  (not in a tick method)", "  listed  evidence: "} {
		if !strings.Contains(out, s) {
			t.Errorf("log --cluster lacks %q:\n%s", s, out)
		}
	}
	if testing.Verbose() {
		t.Log("\n" + out)
	}
	st, _ := e.App.Loadout()
	imp := e.App.ImplicatedMods(st)
	if slices.Contains(imp, byPkg(t, st, "tester.moda")) || !slices.Contains(imp, byPkg(t, st, "tester.remover")) {
		t.Errorf("ImplicatedMods %v", imp)
	}
	out = e.MustRun("harmony", "--target", "Verse.Thing.Destroy")
	if !strings.Contains(out, "dynamic patches the newest log shows on this method") || !strings.Contains(out, "CompThrower.Dyn_Computed.Prefix") || strings.Contains(out, "no declared patches") {
		t.Errorf("harmony --target:\n%s", out)
	}
	out = e.MustRun("harmony", "--target", "Verse.Thing.Nothing")
	if !strings.Contains(out, "target at run time (TargetMethod())") {
		t.Errorf("harmony --target (no log row):\n%s", out)
	}
	if testing.Verbose() {
		t.Log("\n" + out + "\n" + e.MustRun("harmony", "--mod", "tester.thrower"))
	}
}
