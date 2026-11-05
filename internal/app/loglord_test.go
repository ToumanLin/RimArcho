package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// twoModEnv: mods Adder (ModA.dll, tester.adder) and Holder (ModB.dll, tester.holder), scanned and analysed.
func twoModEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	mods := filepath.Join(e.Game, "Mods")
	for _, m := range []struct{ dir, pkg, dll string }{{"Adder", "tester.adder", "ModA.dll"}, {"Holder", "tester.holder", "ModB.dll"}} {
		e.WriteMod(filepath.Join(mods, m.dir), testkit.Mod{PackageID: m.pkg, Name: m.dir, Author: "Tester"})
		b, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), m.dll))
		if err != nil {
			t.Fatal(err)
		}
		e.WriteFile(filepath.Join(mods, m.dir, "Assemblies", m.dll), string(b))
	}
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.adder", "tester.holder")
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	return e
}

// Lord.AddPawn without a stack (1.6): the mod whose lord adds the pawn is the source, the lord
// the pawn is already in is only mentioned, though the game glues "lord job" to both names.
func TestLordAddPawnBlamesAdder(t *testing.T) {
	t.Parallel()
	e := twoModEnv(t)
	const msg = "Tried to add pawn Ann to lord with lord jobModA.Boot but this pawn is already a member of lord with lord jobModB.Boot. Pawns can't be members of more than one lord at the same time."
	rep, err := e.App.AnalyzeLogText("RimWorld 1.6.4633 rev1261\n" + msg + "\n")
	if err != nil {
		t.Fatal(err)
	}
	roles, text := ruleCluster(t, e, rep, "Tried to add pawn")
	if roles["tester.adder"] != "source" || roles["tester.holder"] != "mentioned" {
		t.Errorf("roles %v, want adder source and holder mentioned\n%s", roles, text)
	}
	for _, lc := range rep.Clusters {
		if lc.Class != "game-error" {
			t.Errorf("class %q, want game-error", lc.Class)
		}
	}
}

// Unity's "Type:Method (args)" stacks (1.6): Unity's own exception stack names the mod's frame,
// and the stack Unity adds to every message when full stack traces are on makes no clusters of
// its own (before, "Verse.Log:Error (string)" was one).
func TestUnityStackFrames(t *testing.T) {
	t.Parallel()
	e := twoModEnv(t)
	const log = "RimWorld 1.6.4633 rev1261\n" +
		"Initializing new game\n" +
		"UnityEngine.StackTraceUtility:ExtractStackTrace ()\n" +
		"UnityEngine.Debug:LogError (object)\n" +
		"Verse.Log:Error (string)\n" +
		"Verse.Root/<>c:<Start>b__10_1 ()\n" +
		"\n" +
		"NullReferenceException: Object reference not set to an instance of an object\n" +
		"[Ref 8764AD50] Duplicate stacktrace, see ref for original\n" +
		"UnityEngine.GameObject:AddComponent(Type)\n" +
		"ModA.Boot:<Make>b__3_0()\n" +
		"Verse.TickManager:DoSingleTick()\n"
	rep, err := e.App.AnalyzeLogText(log)
	if err != nil {
		t.Fatal(err)
	}
	roles, text := ruleCluster(t, e, rep, "NullReferenceException")
	if roles["tester.adder"] != "source" || roles["tester.holder"] != "" {
		t.Errorf("roles %v, want adder source\n%s", roles, text)
	}
	if len(rep.Clusters) != 1 {
		for _, lc := range rep.Clusters {
			t.Errorf("cluster %s", lc.Key)
		}
	}
}
