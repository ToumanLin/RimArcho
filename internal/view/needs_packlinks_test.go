package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// K12 (08 §5.3): a pack's links left in Mods/ are one item in 要你确认, counted, with one
// action that removes the links; the home page's launch tab says it.
func TestPackLinksNeed(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("pack", "build", "first")
	e.MustRun("pack", "launch", "first")

	build := func() *view.Needs {
		b, err := view.BuildBoard(e.App)
		if err != nil {
			t.Fatal(err)
		}
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			t.Fatal(err)
		}
		return view.BuildNeeds(e.App, b.St, ps)
	}
	n := build()
	var it *view.Need
	for i := range n.Items {
		if strings.Contains(n.Items[i].Text, "left its links") {
			it = &n.Items[i]
		}
	}
	if it == nil || it.Section != view.SectionOf(view.GroupConfirm) || !it.Counted || len(it.Actions) != 1 || it.Actions[0].Kind != view.ActUnlinkPacks || it.Actions[0].Confirm != "" {
		t.Fatalf("item: %+v", it)
	}
	if view.PackLinksText(n) == "" || view.BreakCount(n) != 0 {
		t.Fatalf("home: %q, breaks %d", view.PackLinksText(n), view.BreakCount(n))
	}
	if _, err := view.Do(e.App, it.Actions[0], 0); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(filepath.Join(e.Game, "Mods"))
	for _, de := range ents {
		if strings.HasPrefix(de.Name(), "!ArchoPack_") {
			t.Fatalf("%s is still there", de.Name())
		}
	}
	if view.PackLinksText(build()) != "" {
		t.Fatal("the item is still there after removing the links")
	}
}
