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

// Two folders of one packageId in Mods/ (08 §3): an item in 会让游戏出错; with RimArcho's link
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
			if it.Section != view.SectionOf(view.GroupList) || !it.Counted || len(it.Actions) != 1 {
				t.Fatalf("item: %+v", it)
			}
			kinds[it.Title] = it.Actions[0].Kind
		}
	}
	if kinds["Mods/ holds tester.same twice"] != view.ActList {
		t.Fatalf("the player's folders: %v", kinds)
	}
	if k := kinds["Mods/ holds tester.cmd twice"]; k != view.ActSwap && k != view.ActDisable {
		t.Fatalf("RimArcho's link: %v", kinds)
	}
}
