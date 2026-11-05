package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Entries the list has but nothing installed matches (an imported list) are one item with
// one action for all of them, and a mod needing one of them adds no item of its own (16 §5).
func TestMissingEntriesAreOneItem(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	e.AddWorkshop("3000000001", testkit.Mod{PackageID: "tester.user", Name: "User", Deps: []string{"tester.gone1"}})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.gone1", "tester.gone2", "tester.user")
	e.AddEnv(false)
	e.MustRun("scan")

	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := view.BuildNeeds(e.App, b.St, ps)
	var missing []view.Need
	for _, it := range n.Items {
		if strings.Contains(it.Text, "not installed") {
			missing = append(missing, it)
		}
	}
	if len(missing) != 1 {
		t.Fatalf("want one item for the missing entries, got %d: %+v", len(missing), missing)
	}
	it := missing[0]
	if it.Text != "2 mods in the list are not installed" || !it.Fold || !it.Counted || len(it.Parts) != 2 {
		t.Fatalf("item: %+v", it)
	}
	var kinds []string
	for _, a := range it.Actions {
		kinds = append(kinds, a.Kind)
	}
	if strings.Join(kinds, " ") != "disable fold" {
		t.Fatalf("actions (no Workshop IDs known, so no subscribe): %v", kinds)
	}
	if got := it.Actions[0].Refs; len(got) != 2 {
		t.Fatalf("remove refs: %v", got)
	}
	if _, err := view.Do(e.App, it.Actions[0], 0); err != nil {
		t.Fatal(err)
	}
	b, _ = view.BuildBoard(e.App)
	for _, en := range b.St.List {
		if en.Missing != "" {
			t.Fatalf("still in the list: %s", en.Missing)
		}
	}
}
