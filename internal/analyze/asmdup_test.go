package analyze

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/harmony"
)

// Types the compiler adds (EmbeddedAttribute, RefSafetyRulesAttribute) are not lost when a
// later copy has them and the loaded copy does not: the finding is info, not a warning
// (VM round 2026-11-05, h2 after moving Outfit Stands Plus first).
func TestAsmDuplicatesIgnoresCompilerTypes(t *testing.T) {
	t.Parallel()
	active := []*Mod{{Instance: "a", Pos: 1}, {Instance: "b", Pos: 2}}
	asm := func(inst string, types ...string) *AssemblyFacts {
		return &AssemblyFacts{Instance: inst, File: "1.6/Assemblies/Shared.dll", Assembly: &harmony.Assembly{Types: types}}
	}
	fs := asmDuplicates(active, []*AssemblyFacts{
		asm("a", "Shared.Thing", "Shared.Comp"),
		asm("b", "Shared.Thing", "Shared.Comp", "Microsoft.CodeAnalysis.EmbeddedAttribute", "System.Runtime.CompilerServices.RefSafetyRulesAttribute"),
	})
	if len(fs) != 1 || fs[0].Severity != finding.Info {
		t.Fatalf("want one info finding, got %+v", fs)
	}
	fs = asmDuplicates(active, []*AssemblyFacts{
		asm("a", "Shared.Thing"),
		asm("b", "Shared.Thing", "Shared.Comp", "Microsoft.CodeAnalysis.EmbeddedAttribute"),
	})
	if len(fs) != 1 || fs[0].Severity != finding.Warning || fs[0].Claim == "" {
		t.Fatalf("want a warning for Shared.Comp, got %+v", fs)
	}
}
