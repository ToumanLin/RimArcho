package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// P3 (04 §1.6): the game now is not among the versions the list allows: one counted item in
// for you to confirm; "add 1.6" allows it and the item goes. Launching is not stopped by it.
func TestGameNotAllowedNeed(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	e.AddEnv(false)
	e.MustRun("scan")

	find := func() *view.Need {
		b, err := view.BuildBoard(e.App)
		if err != nil {
			t.Fatal(err)
		}
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			t.Fatal(err)
		}
		n := view.BuildNeeds(e.App, b.St, ps)
		for i := range n.Items {
			if strings.Contains(n.Items[i].Text, "the game now is") {
				return &n.Items[i]
			}
		}
		return nil
	}
	if it := find(); it != nil {
		t.Fatalf("the default allows the game: %+v", it)
	}
	e.MustRun("plan", "versions", "default", "1.4-1.5")
	it := find()
	if it == nil || it.Text != "default allows 1.4–1.5; the game now is 1.6" || it.Section != view.SectionOf(view.GroupConfirm) || !it.Counted ||
		len(it.Actions) != 2 || it.Actions[0].Kind != view.ActAddVersion || it.Actions[1].Kind != view.ActManageLists {
		t.Fatalf("item: %+v", it)
	}
	done, err := view.Do(e.App, it.Actions[0], 0)
	if err != nil || done != "default: game versions 1.4–1.6" {
		t.Fatalf("add: %q, %v", done, err)
	}
	if it := find(); it != nil {
		t.Fatalf("still there: %+v", it)
	}
}
