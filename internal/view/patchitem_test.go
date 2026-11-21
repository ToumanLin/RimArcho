package view_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 16 §5, 05 §4.4: patches that find nothing without an inactive target are one quiet item
// per Mod in "worth knowing", with a row mark, not counted; once the current game log
// shows a patch of that Mod failing, the item is a counted warning.
func TestPatchesThatFindNothingMarkTheirMod(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	mods := filepath.Join(e.Game, "Mods")
	plain := func(pkg, name string, files map[string]string) testkit.Mod {
		return testkit.Mod{PackageID: pkg, Name: name, Author: "Tester", Files: files}
	}
	e.WriteMod(filepath.Join(mods, "Lib"), plain("tester.lib", "Lib", map[string]string{
		"Defs/Lib.xml": "<Defs>\n  <ThingDef><defName>Lib_Gun</defName></ThingDef>\n</Defs>\n"}))
	stalePatch := filepath.Join(mods, "Stale", "Patches", "Stale.xml")
	e.WriteMod(filepath.Join(mods, "Stale"), plain("tester.stale", "Stale", map[string]string{
		"Patches/Stale.xml": "<Patch>\n" +
			"  <Operation Class=\"PatchOperationAdd\">\n    <xpath>Defs/ThingDef[defName=\"Lib_Gun\"]/comps</xpath>\n    <value><li>x</li></value>\n  </Operation>\n" +
			"  <Operation Class=\"PatchOperationAdd\">\n    <xpath>Defs/ThingDef[defName=\"Lib_Gone\"]</xpath>\n    <value><x/></value>\n  </Operation>\n" +
			"</Patch>\n"}))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.lib", "tester.stale")
	os.Chtimes(e.ModsConfig, e.Now().Add(-time.Hour), e.Now().Add(-time.Hour))
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	needs := func() (*view.Needs, string) {
		t.Helper()
		e.App.LazyLog, e.App.LogCacheOnly = false, false
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			t.Fatal(err)
		}
		n := view.BuildNeeds(e.App, ps.State, ps)
		id := ""
		for _, m := range ps.State.Ix.Mods {
			if m.PackageID == "tester.stale" {
				id = m.ID
			}
		}
		return n, id
	}
	item := func(n *view.Needs) *view.Need {
		for i := range n.Items {
			if n.Items[i].Kind == "patch.no-match" {
				return &n.Items[i]
			}
		}
		return nil
	}
	n, id := needs()
	it := item(n)
	if it == nil {
		t.Fatalf("no item for Stale's patches: %+v", n.Items)
	}
	if !strings.Contains(it.Title, "2 patches of Stale find nothing") || it.Counted || it.Status != view.StatusHint || it.Section != view.SectionHint {
		t.Fatalf("quiet item: %+v", *it)
	}
	if n.Status[id] != view.StatusHint || n.Count != 0 {
		t.Fatalf("row mark %d, count %d", n.Status[id], n.Count)
	}
	// the game logged one of them failing: a counted warning that says so
	e.RecordLaunch(e.Now().Add(-30 * time.Minute))
	logPath := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(logPath, fmt.Sprintf("Mono path[0] = 'x'\nRimWorld 1.6.4633 rev1261\n\n"+
		"[Stale] Patch operation Verse.PatchOperationAdd(Defs/ThingDef[defName=\"Lib_Gone\"]) failed\nfile: %s\n\n", stalePatch))
	os.Chtimes(logPath, e.Now(), e.Now())
	n, _ = needs()
	it = item(n)
	if it == nil || !it.Counted || it.Status != view.StatusWarn || !strings.HasPrefix(it.Why, "it failed in the last run") {
		t.Fatalf("promoted item: %+v", it)
	}
}

// Many Mods with patches that find nothing (an old list on a new game) are one folded
// item; each row keeps its own title.
func TestPatchesThatFindNothingFoldForManyMods(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	mods := filepath.Join(e.Game, "Mods")
	pkgs := []string{"ludeon.rimworld"}
	for i := 0; i < 5; i++ {
		pkg := fmt.Sprintf("tester.old%d", i)
		e.WriteMod(filepath.Join(mods, fmt.Sprintf("Old%d", i)), testkit.Mod{PackageID: pkg, Name: fmt.Sprintf("Old %d", i), Author: "Tester", Files: map[string]string{
			"Patches/Old.xml": fmt.Sprintf("<Patch>\n  <Operation Class=\"PatchOperationAdd\">\n    <xpath>Defs/ThingDef[defName=\"Gone_%d\"]</xpath>\n    <value><x/></value>\n  </Operation>\n</Patch>\n", i)}})
		pkgs = append(pkgs, pkg)
	}
	e.WriteModsConfig("1.6.4633 rev1261", pkgs...)
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := view.BuildNeeds(e.App, ps.State, ps)
	var items []view.Need
	for _, it := range n.Items {
		if it.Kind == "patch.no-match" {
			items = append(items, it)
		}
	}
	if len(items) != 1 || items[0].Title != "patches of 5 Mods find nothing" || len(items[0].Mods) != 5 || items[0].Counted {
		t.Fatalf("folded item: %+v", items)
	}
	for _, id := range items[0].Mods {
		if rs := n.Reasons[id]; len(rs) != 1 || !strings.HasPrefix(rs[0].Text, "a patch of Old") || n.Status[id] != view.StatusHint {
			t.Fatalf("row %s: %+v", id, rs)
		}
	}
}
