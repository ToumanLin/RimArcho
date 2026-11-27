package app_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 10 §2: the build check counts Def and patch errors of the list's own analysis; the run
// of another list, which Can it run shows for a list never analysed, does not block it
// (release round of 11-27: "63 things to fix first", all of another list's Mods).
func TestPackBuildIgnoresAnotherListsAnalysis(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "test.child", Name: "Child", Files: map[string]string{
		"Defs/Child.xml": `<Defs><HediffDef ParentName="Base"><defName>Illness</defName></HediffDef></Defs>`}})
	e.AddWorkshop("1002", testkit.Mod{PackageID: "test.plain", Name: "Plain", Files: map[string]string{
		"Defs/Plain.xml": `<Defs><HediffDef><defName>Fine</defName></HediffDef></Defs>`}})
	e.WriteModsConfig("1.6", "ludeon.rimworld", "test.child", "test.plain")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	if out, code := e.Run("pack", "build", "mine", "--dry-run"); code == 0 || !strings.Contains(out, "things to fix first") || !strings.Contains(out, "Base") {
		t.Fatalf("the list's own Def error does not block: exit %d\n%s", code, out)
	}
	e.MustRun("disable", "pkg:test.child") // another Mod set, never analysed
	out, code := e.Run("pack", "build", "mine", "--dry-run")
	if code != 0 || !strings.Contains(out, "dry run, nothing written") || !strings.Contains(out, "has not been analysed") {
		t.Fatalf("another list's run blocks the build: exit %d\n%s", code, out)
	}
	e.MustRun("analyze")
	if out := e.MustRun("pack", "build", "mine", "--dry-run"); strings.Contains(out, "has not been analysed") {
		t.Fatalf("the note stays after the analysis:\n%s", out)
	}
}
