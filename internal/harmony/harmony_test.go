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
	if !m.Manual || len(m.Patches) != 0 || !slices.Contains(m.IDs, "tester.manual") {
		t.Errorf("ModManual: manual=%v patches=%d ids=%v", m.Manual, len(m.Patches), m.IDs)
	}
	a, _ := Read(fixture("ModA.dll"), false)
	if a.Manual || !slices.Contains(a.IDs, "tester.moda.harmony") || !slices.Contains(a.Types, "ModA.CompBroken") {
		t.Errorf("ModA ids=%v types=%v", a.IDs, a.Types)
	}
	if !Skipped("0Harmony.dll", DefaultSkip) || !Skipped("System.Runtime.dll", DefaultSkip) || Skipped("ModA.dll", DefaultSkip) {
		t.Error("skip list")
	}
	if !strings.HasPrefix(fixture("x"), "") {
		t.Fatal()
	}
}
