package view_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Hidden mods are listed on Can it run but not counted in the sidebar's number (16 §2.1).
func TestHiddenModsNotCounted(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	dir := e.AddWorkshop("1002", testkit.Sample("tester.h", "Hidden One"))
	e.Move(filepath.Join(dir, "About", "About.xml"), filepath.Join(dir, "About", "About.xml.disabled"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
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
	hidden, other := 0, 0
	for _, it := range n.Items {
		if !it.Counted {
			continue
		}
		if it.Group == view.GroupHidden {
			hidden++
		} else {
			other++
		}
	}
	if hidden == 0 {
		t.Fatalf("no hidden-mod item: %+v", n.Items)
	}
	if n.Count != other {
		t.Fatalf("count %d, want %d (the %d hidden-mod items are not counted)", n.Count, other, hidden)
	}
	// Not on Can it run (16 §2.4): Mod library handles them, one line points there.
	for _, it := range n.Items {
		if it.Group == view.GroupHidden && it.Section != view.SectionElsewhere {
			t.Errorf("hidden-mod item on the page: %+v", it)
		}
	}
	if len(n.Elsewhere) != 1 || n.Elsewhere[0].Page != "library" || n.Elsewhere[0].Count != hidden || n.Elsewhere[0].Query == "" {
		t.Fatalf("elsewhere: %+v", n.Elsewhere)
	}
}
