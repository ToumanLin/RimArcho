package store

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/harmony"
)

// A patch found at a Harmony.Patch call keeps that in the stored facts (05 §5.3, G9): with
// its target resolved it is a manual patch, without it is dynamic too and says why.
func TestHarmonyManualFlag(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "rimarcho.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.MM = "1.6"
	asm := &analyze.AssemblyFacts{Instance: "inst", File: "Assemblies/M.dll", Assembly: &harmony.Assembly{Manual: true, Patches: []harmony.Patch{
		{PatchType: "M.A", PatchMethod: "Prefix", Kind: harmony.Prefix, TargetType: "Verse.Pawn", TargetMethod: "Tick"},
		{PatchType: "M.B", PatchMethod: "After", Kind: harmony.Postfix, TargetType: "Verse.Thing", TargetMethod: "SpawnSetup", Manual: true},
		{PatchType: "M.C", PatchMethod: "Fix", Kind: harmony.Transpiler, Manual: true, Dynamic: true, Unresolved: "parameter"},
	}}}
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := saveContent(tx, &analyze.Output{Content: map[string]*analyze.ContentState{"inst": {Key: "manual", AssemblyKey: "manual-dll"}}, NewDefs: map[string][]analyze.DefFact{"inst": nil}, Assemblies: []*analyze.AssemblyFacts{asm}}, "1.6"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := s.HarmonyPatches(HarmonyFilter{Instance: "inst"})
	if err != nil || len(rows) != 3 {
		t.Fatalf("%v %d rows", err, len(rows))
	}
	got := map[string][2]bool{}
	for _, r := range rows {
		got[r.PatchType] = [2]bool{r.Manual, r.Dynamic}
	}
	want := map[string][2]bool{"M.A": {false, false}, "M.B": {true, false}, "M.C": {true, true}}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: manual, dynamic = %v, want %v", k, got[k], w)
		}
	}
	for _, r := range rows {
		if r.PatchType == "M.C" && r.Target != "<manual: parameter>" {
			t.Errorf("unresolved target %q", r.Target)
		}
	}
}
