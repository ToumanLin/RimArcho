package cmds_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// TestHarmonyOutsideList: harmony --mod answers for a Mod outside the current list, from an
// earlier analysis of that copy when there is one, otherwise from its assemblies read now; the
// shared view names its Mods (05 §5.3, 14 §2).
func TestHarmonyOutsideList(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("disable", ref(e, "Mod C"))
	e.MustRun("analyze")
	out := e.MustRun("harmony", "--mod", ref(e, "Mod C"))
	if !strings.Contains(out, "scope: not in the current list: its assemblies read now for 1.6, not stored") ||
		!strings.Contains(out, "Verse.Pawn.Tick") || strings.Contains(out, "not analysed") {
		t.Fatalf("harmony --mod outside the list, never analysed:\n%s", out)
	}
	// Mod C in the list once, then out again: the stored facts of that copy answer.
	e.MustRun("enable", ref(e, "Mod C"))
	e.MustRun("analyze")
	e.MustRun("disable", ref(e, "Mod C"))
	out = e.MustRun("harmony", "--mod", ref(e, "Mod C"), "--shared")
	if !strings.Contains(out, "scope: not in the current list: from an earlier analysis of this copy for 1.6") ||
		!strings.Contains(out, "Verse.Pawn.Tick") || !strings.Contains(out, "Mod C") || strings.Contains(out, "#?") {
		t.Fatalf("harmony --mod outside the list, analysed before:\n%s", out)
	}
	if out := e.MustRun("harmony"); strings.Contains(out, "Mod C") {
		t.Fatalf("shared targets list a Mod outside the list:\n%s", out)
	}
}
