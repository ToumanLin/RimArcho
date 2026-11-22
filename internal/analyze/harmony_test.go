package analyze

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/harmony"
)

// Every Harmony finding and the branches that build them, pinned to their exact text.
func TestHarmonyFindings(t *testing.T) {
	a := &Mod{Instance: "a", Pkg: "test.a", Pos: 3}
	b := &Mod{Instance: "b", Pkg: "test.b", Pos: 1}
	c := &Mod{Instance: "c", Pkg: "test.c", Pos: 2}
	p := func(kind, typ, method, target string, skips bool) harmony.Patch {
		i := strings.LastIndexByte(target, '.')
		return harmony.Patch{Kind: kind, PatchType: typ, PatchMethod: method, TargetType: target[:i], TargetMethod: target[i+1:], Skips: skips}
	}
	asm := func(inst, file string, ps ...harmony.Patch) *AssemblyFacts {
		return &AssemblyFacts{Instance: inst, File: file, Assembly: &harmony.Assembly{Patches: ps}}
	}
	asms := []*AssemblyFacts{
		asm("a", "Assemblies/A.dll",
			p(harmony.Prefix, "A.P", "Pre", "Verse.Pawn.Tick", true),
			p(harmony.Transpiler, "A.T", "Tr", "Verse.Pawn.Tick", false),
			p(harmony.Postfix, "A.Only", "Post", "Verse.Thing.SpawnSetup", false),
			harmony.Patch{Kind: harmony.Prefix, PatchType: "A.Dyn", PatchMethod: "Pre", Dynamic: true},
		),
		asm("b", "Assemblies/B.dll",
			p(harmony.Transpiler, "B.T", "Tr", "Verse.Pawn.Tick", false),
			p(harmony.Postfix, "B.Post", "Post", "RimWorld.Need.NeedInterval", false),
			p(harmony.Transpiler, "B.T2", "Tr", "RimWorld.Need.NeedInterval", false),
		),
		asm("c", "Assemblies/C.dll",
			p(harmony.Postfix, "C.Post", "Post", "Verse.Pawn.Tick", false),
			p(harmony.Prefix, "C.Pre", "Pre", "RimWorld.Need.NeedInterval", true),
		),
		asm("c", "Assemblies/0Harmony.dll", p(harmony.Prefix, "H", "Pre", "Verse.Thing.SpawnSetup", false)),
		asm("gone", "Assemblies/G.dll", p(harmony.Prefix, "G", "Pre", "Verse.Thing.SpawnSetup", false)),
	}
	asms[3].Skipped = true
	asms[0].Manual, asms[0].ManualCalls, asms[0].ManualResolved = true, 3, 1
	asms[0].Unresolved = map[string]int{"field": 1, "arg": 1}
	asms = append(asms, asm("a", "Assemblies/A2.dll"), asm("b", "Assemblies/B2.dll"))
	asms[5].Manual, asms[5].ManualCalls, asms[5].ManualResolved = true, 2, 1
	asms[5].Unresolved = map[string]int{"field": 1}
	asms[6].Manual = true

	out, err := json.MarshalIndent(harmonyFindings([]*Mod{a, b, c}, asms), "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got := string(out) + "\n"
	want, err := os.ReadFile("testdata/harmony_findings.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("findings changed:\n%s", got)
	}
}
