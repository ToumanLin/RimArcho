package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Lord.AddPawn without a stack (1.6): the mod whose lord adds the pawn is the source, the lord
// the pawn is already in is only mentioned, though the game glues "lord job" to both names.
func TestLordAddPawnBlamesAdder(t *testing.T) {
	t.Parallel()
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
