package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The last run read a save that names a def of a mod left out of the list: the item names
// that mod, says it is not in the list, and does not call the source unconfirmed (16 §5;
// VM run 2026-10-28, where a patcher of Pawn.ExposeData was named instead).
func TestLastRunNamesLeftOutMod(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	log := e.HarmonyLogFixture() + "Could not load reference to Verse.ThingDef named Lib_Gun\n" +
		"  at (wrapper dynamic-method) Verse.Pawn.Verse.Pawn.ExposeData_Patch1(Verse.Pawn)\n" +
		"    - POSTFIX tester.modb: Void ModB.Pawn_Tick_B:Postfix()\n\n"
	p := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(p, log)
	os.Chtimes(p, e.Now(), e.Now())
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range view.BuildNeeds(e.App, st, ps).Items {
		if it.Group != view.GroupLastRun {
			continue
		}
		got = append(got, it.Text)
		if strings.HasPrefix(it.Title, "Lib — ") {
			if !strings.HasSuffix(it.Why, "; it is not in the list") || strings.Contains(it.Why, "source not confirmed") || !strings.Contains(it.Text, it.Why) {
				t.Fatalf("item %+v", it)
			}
			return
		}
	}
	t.Fatalf("no item for Lib: %q", got)
}
