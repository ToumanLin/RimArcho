package app_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// staticCtorLog is the MCP eval c3 error in the fixture's names: a Harmony patch whose
// target is gone fails inside the mod's static constructor.
const staticCtorLog = "Error in static constructor of ModA.Boot: System.TypeInitializationException: The type initializer for 'ModA.Boot' threw an exception. ---> HarmonyLib.HarmonyException: Patching exception in method null ---> System.ArgumentException: Undefined target method for patch method static System.Boolean ModA.Pawn_Tick::Prefix(Verse.Pawn __instance)\n"

// codeModEnv has Mod A (fixture ModA.dll, which defines ModA.Boot) enabled and was only
// scanned: no analysis has run yet, as in a freshly staged environment.
func codeModEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := testRunEnv(t)
	dir := filepath.Join(e.Game, "Mods", "ModA")
	e.WriteMod(dir, testkit.Mod{PackageID: "tester.moda", Name: "Mod A", Author: "Tester"})
	dll, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), "ModA.dll"))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(dir, "Assemblies", "ModA.dll"), string(dll))
	e.MustRun("scan")
	e.MustRun("enable", "pkg:tester.moda")
	return e
}

// A static constructor failure names the mod whose assembly defines the type, even when
// the test run is the first command after a scan (03 §6.2, 08 §12).
func TestTestRunAttributesTypeToItsAssembly(t *testing.T) {
	t.Parallel()
	e := codeModEnv(t)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if s, err := e.App.Analysis(st); err != nil || s.Run != nil {
		t.Fatalf("fixture already analysed: %+v, %v", s, err)
	}
	e.App.TestGameStart = fakeTestGame(t, "menu", staticCtorLog)
	var progress []string
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{Timeout: 5 * time.Second, Settle: time.Millisecond,
		Progress: func(s string) { progress = append(progress, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Clusters) != 1 || !slices.Equal(r.Clusters[0].Mods, []string{"Mod A"}) || r.Clusters[0].Confidence != finding.Likely || r.AnalysisError != "" {
		t.Fatalf("clusters %+v, analysis error %q", r.Clusters, r.AnalysisError)
	}
	if !slices.Contains(progress, "analysing the plan's mods for attribution") {
		t.Fatalf("progress %v", progress)
	}
	// The second run finds the analysis fresh and does not run it again.
	runs := 0
	e.App.AnalyzeRunner = func(string, func(string)) error { runs++; return nil }
	if _, err := e.App.TestRun(context.Background(), app.TestRunOptions{Timeout: 5 * time.Second, Settle: time.Millisecond}); err != nil || runs != 0 {
		t.Fatalf("second run analysed %d times: %v", runs, err)
	}
}

// `archo log` (MCP analyze_log) brings the analysis up to date for the environment's own
// log, so the same error names the same mod there.
func TestLogAttributesTypeToItsAssembly(t *testing.T) {
	t.Parallel()
	e := codeModEnv(t)
	e.RecordLaunch(e.Now().Add(-30 * time.Minute))
	p := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(p, "RimWorld 1.6.4633 rev1261\n"+staticCtorLog)
	os.Chtimes(p, e.Now(), e.Now())
	out := e.MustRun("log")
	if !strings.Contains(out, "(source)") || !strings.Contains(out, "exception") {
		t.Fatalf("log output:\n%s", out)
	}
	out = e.MustRun("log", "--cluster", "E1")
	if !strings.Contains(out, "source") || !strings.Contains(out, "static constructor") || !strings.Contains(out, "log message names type ModA.Boot") {
		t.Fatalf("cluster output:\n%s", out)
	}
}
