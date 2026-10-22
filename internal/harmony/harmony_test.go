package harmony

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func fixture(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "dotnet", "bin", name)
}

func patches(t *testing.T, dll string) []string {
	t.Helper()
	a, err := Read(fixture(dll), false)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range a.Patches {
		s := p.Kind + " " + p.Target() + " <- " + p.PatchType + "." + p.PatchMethod
		if p.Skips {
			s += " skips"
		}
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

func TestReadFixtures(t *testing.T) {
	if got, want := patches(t, "ModA.dll"), []string{
		"postfix Verse.Thing.get_HitPoints <- ModA.Thing_HitPoints.After",
		"prefix Verse.Pawn.Tick <- ModA.Pawn_Tick.Prefix skips",
	}; !slices.Equal(got, want) {
		t.Errorf("ModA = %q", got)
	}
	if got, want := patches(t, "ModB.dll"), []string{
		"postfix Verse.Pawn.Tick <- ModB.Pawn_Tick_B.Postfix",
		"prefix Verse.Pawn.Kill(int) <- ModB.Pawn_Kill.Prefix",
		"transpiler Verse.Pawn.Tick <- ModB.Pawn_Tick_B.Transpiler",
	}; !slices.Equal(got, want) {
		t.Errorf("ModB = %q", got)
	}
	if got := patches(t, "ModC.dll"); !slices.Equal(got, []string{"transpiler Verse.Pawn.Tick <- ModC.Pawn_Tick_C.Rewrite"}) {
		t.Errorf("ModC = %q", got)
	}
	m, err := Read(fixture("ModManual.dll"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Manual || len(m.Patches) != 1 || !m.Patches[0].Manual || !slices.Contains(m.IDs, "tester.manual") {
		t.Errorf("ModManual: manual=%v patches=%v ids=%v", m.Manual, m.Patches, m.IDs)
	}
	if got := patches(t, "ModManual.dll"); !slices.Equal(got, []string{"postfix Verse.ThingWithComps.Tick <- ModManual.Boot.After"}) {
		t.Errorf("ModManual = %q", got)
	}
	a, _ := Read(fixture("ModA.dll"), false)
	if a.Manual || !slices.Contains(a.IDs, "tester.moda.harmony") || !slices.Contains(a.Types, "ModA.CompBroken") {
		t.Errorf("ModA ids=%v types=%v", a.IDs, a.Types)
	}
	if !SkippedFor("HugsLib.dll", "OtherMod", DefaultSkip) || SkippedFor("HugsLib.dll", "UnlimitedHugs.HugsLib_steam", DefaultSkip) {
		t.Error("HugsLib.dll is read in its own mod only")
	}
	if !Skipped("0Harmony.dll", DefaultSkip) || !Skipped("System.Runtime.dll", DefaultSkip) || Skipped("ModA.dll", DefaultSkip) {
		t.Error("skip list")
	}
	if !strings.HasPrefix(fixture("x"), "") {
		t.Fatal()
	}
}

// One Harmony.Patch call per pattern in testdata/dotnet/ModManualIL/P.cs.
func TestManualPatchTargets(t *testing.T) {
	want := []string{
		"finalizer Verse.Pawn..ctor <- ModIL.Patches.Fin",                  // AccessTools.Constructor
		"finalizer Verse.Pawn.Kill <- ModIL.Patches.Fin",                   // foreach over a List
		"finalizer Verse.Thing.Tick <- ModIL.Patches.Fin",                  // (second element)
		"postfix <manual: computed name> <- ModIL.Patches.Fin",             // "Ki" + Environment.TickCount
		"postfix ModIL.Patches.Post <- ModIL.Patches.Post",                 // SymbolExtensions (ldtoken), implicit HarmonyMethod
		"postfix Verse.Pawn.Kill <- ModIL.Patches.Fin",                     // LINQ GetMethods().Where(x => x.Name.Contains("Kill")).First()
		"postfix Verse.Pawn.Kill <- ModIL.Patches.Post",                    // AccessTools.TypeByName
		"postfix Verse.Pawn.Tick <- ModIL.Patches.Post",                    // a helper of the mod, called with constants
		"postfix Verse.Thing.Tick <- ModIL.Patches.Post",                   // "Type:Name" through a local, Type.GetMethod
		"postfix Verse.Thing.get_HitPoints <- ModIL.Patches.Post",          // PropertyGetter, HarmonyMethod(Type, string)
		"postfix Verse.ThingWithComps.get_HitPoints <- ModIL.Patches.Post", // GetProperty().GetGetMethod(), delegate
		"prefix Verse.Pawn.Kill <- ModIL.Patches.Pre skips",                // static field holding the patch type
		"prefix Verse.Pawn.Kill(int) <- ModIL.Patches.Pre skips",           // GetMethod with Type[]
		"prefix Verse.Pawn.Tick <- ModIL.OptA.Pre skips",                   // virtual DoPatches, AccessTools.Method(GetType(), name), ternaries
		"prefix Verse.Thing.Tick <- ModIL.Patches.Pre skips",               // foreach over an array
		"prefix Verse.ThingWithComps.Tick <- ModIL.Patches.Pre skips",
		"transpiler <manual: parameter> <- ModIL.Patches.Trans",       // a public helper nobody calls
		"transpiler Verse.Thing.set_HitPoints <- ModIL.Patches.Trans", // PropertySetter, AccessTools.Method patch
	}
	if got := patches(t, "ModManualIL.dll"); !slices.Equal(got, want) {
		t.Errorf("ModManualIL:\n got %q\nwant %q", got, want)
	}
	a, err := Read(fixture("ModManualIL.dll"), false)
	if err != nil {
		t.Fatal(err)
	}
	if a.ManualCalls != 17 || a.ManualResolved != 14 || a.Unresolved["computed name"] != 1 || a.Unresolved["parameter"] != 2 {
		t.Errorf("calls %d resolved %d unresolved %v", a.ManualCalls, a.ManualResolved, a.Unresolved)
	}
	// The wrapper's own call has parameters only; another assembly calling it is resolved.
	if got := patches(t, "ModWrapperUser.dll"); !slices.Equal(got, []string{"prefix Verse.Thing.Tick <- ModWU.P.Pre skips"}) {
		t.Errorf("ModWrapperUser = %q", got)
	}
	for _, p := range a.Patches {
		if p.Unresolved != "" && !p.Dynamic {
			t.Errorf("unresolved target must be dynamic: %+v", p)
		}
	}
}
