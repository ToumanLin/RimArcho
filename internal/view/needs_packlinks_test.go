package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// K12 (08 §5.3): a pack's links left in Mods/ are one item in for you to confirm, counted, with one
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

// Two folders of one packageId in Mods/ (08 §3): an item in Will cause errors; with RimArcho's link
// among them the action changes the plan, with only the player's folders it lists them.
func TestModsDuplicateNeed(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddEnv(true)
	cmdDir := filepath.Join(e.SteamCMD, "8888")
	e.WriteMod(cmdDir, testkit.Sample("tester.cmd", "CmdOnly"))
	mods := filepath.Join(e.Game, "Mods")
	e.WriteMod(filepath.Join(mods, "P1"), testkit.Sample("tester.same", "Same"))
	e.WriteMod(filepath.Join(mods, "P2"), testkit.Sample("tester.same", "Same"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.MustRun("scan")
	e.MustRun("enable", "pkg:tester.cmd")
	e.MustRun("save")
	e.WriteMod(filepath.Join(mods, "CmdCopy"), testkit.Sample("tester.cmd", "CmdOnly"))
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
	kinds := map[string]string{}
	for _, it := range n.Items {
		if strings.HasPrefix(it.Title, "Mods/ holds") {
			if it.Section != view.SectionOf(view.GroupList) || !it.Counted || len(it.Actions) != 2 {
				t.Fatalf("item: %+v", it)
			}
			kinds[it.Title] = it.Actions[0].Kind
		}
	}
	// move into the lock library first, for the player's folders and beside RimArcho's link alike (08 §3)
	if kinds["Mods/ holds Same twice"] != view.ActMoveLocked || kinds["Mods/ holds CmdOnly twice"] != view.ActMoveLocked {
		t.Fatalf("first actions: %v", kinds)
	}
}
