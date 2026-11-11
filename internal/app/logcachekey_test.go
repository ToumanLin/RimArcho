package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A new analysis of the same slot gets the replaced run's id again (SQLite reuses the
// highest rowid). The stored log report must not survive it: after `archo analyze` a log read
// while two new Mods were not analysed still said so and blamed another Mod (VM 2026-11-10).
func TestLogReportRebuiltAfterReanalysis(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	b, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), "ModA.dll"))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteMod(filepath.Join(e.Game, "Mods", "Other"), testkit.Sample("tester.other", "Other"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.other")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	log := filepath.Join(e.UserData, "Player.log")
	writeLogAt(t, e, log, "RimWorld 1.6.4633 rev1261\n"+
		"Mono path[0] = '"+filepath.ToSlash(filepath.Join(e.Game, "RimWorldWin64_Data", "Managed"))+"'\n"+
		"Error in static constructor of ModA.Boot: System.TypeInitializationException: The type initializer for 'ModA.Boot' threw an exception. ---> HarmonyLib.HarmonyException: Patching exception\n"+
		"  at HarmonyLib.PatchClassProcessor.Patch () [0x00000] in <abc>:0\n", e.Now())
	// Installed after the analysis, as on the VM: it has no analysed content yet.
	e.WriteMod(filepath.Join(e.Game, "Mods", "Failer"), testkit.Mod{PackageID: "tester.failer", Name: "Failer", Author: "Tester",
		Files: map[string]string{"Assemblies/ModA.dll": string(b)}})
	e.MustRun("scan")
	e.MustRun("enable", "pkg:tester.failer")
	// The window reads the log without analysing first: the report stored now is the stale one.
	rep, err := e.App.AnalyzeLog(log)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Unanalysed != 1 {
		t.Fatalf("before the analysis: %d not analysed", rep.Unanalysed)
	}
	e.MustRun("analyze")
	if out := e.MustRun("log"); strings.Contains(out, "not analysed yet") {
		t.Fatalf("stored report kept after the analysis:\n%s", out)
	}
	if cl := e.MustRun("log", "--cluster", "E1"); !strings.Contains(cl, "Failer") || !strings.Contains(cl, "source") {
		t.Fatalf("not attributed after the analysis:\n%s", cl)
	}
}
