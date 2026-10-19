package cmds_test

import (
	"testing"

	"github.com/rimarcho/rimarcho/internal/testkit"
)

// TestGoldenHarmonyLog covers the Harmony layer (C5 multi-transpiler, C6 manual-only) and
// log attribution (B1 patch failure, B2 cross-reference, B3 Pawn.Tick patchers, B4 repeats).
func TestGoldenHarmonyLog(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	runGolden(t, "harmonylog", e, []step{
		cmd("analyze"),
		cmd("harmony"),
		cmd("harmony", "--target", "Verse.Pawn.Tick"),
		func(e *testkit.Env) []string { return []string{"harmony", "--mod", ref(e, "Manual Patcher")} },
		cmd("problems", "--code", "harmony"),
		cmd("log"),
		cmd("log", "--cluster", "E1"),
		cmd("log", "--cluster", "E2"),
		cmd("log", "--cluster", "E3"),
		cmd("log", "--cluster", "E4"),
		cmd("log", "--select"),
		cmd("problems", "--code", "log"),
	})
}
