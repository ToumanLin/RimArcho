package cmds_test

import (
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
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
		func(e *testkit.Env) []string { return []string{"why", ref(e, "Mod B"), ref(e, "Mod C")} },
		cmd("log"),
		cmd("log", "--cluster", "E1"),
		cmd("log", "--cluster", "E2"),
		cmd("log", "--cluster", "E3"),
		cmd("log", "--cluster", "E4"),
		cmd("log", "--select"),
		cmd("problems", "--code", "log"),
		// 03 §6.3: logs are grouped under one row each; a change to the Loadout after the
		// log makes it stale: folded by default and left out of the counts.
		cmd("problems"),
		cmd("status"),
		func(e *testkit.Env) []string {
			e.Advance(time.Hour)
			return []string{"disable", ref(e, "Manual Patcher")}
		},
		cmd("problems"),
		cmd("status"),
		cmd("problems", "--code", "log"),
		cmd("problems", "show", "p:4c087a"),
	})
}
