package app_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestUseInstalledPositionsAndUndo(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	for _, name := range []string{"one", "two"} {
		e.WriteMod(filepath.Join(e.SteamCMD, name), testkit.Sample("tester."+name, name))
	}
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.one", "tester.a", "tester.two", "tester.absent")
	scanNow(t, e)
	before, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	head := e.App.Store.LastStepID(before.Plan.ID, "undo")
	got, err := e.App.UseInstalled([]string{" TESTER.ONE_STEAM ", "tester.two", "tester.one"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.State.List) != len(before.List) {
		t.Fatal("list length changed")
	}
	for i, e := range before.List {
		if e.Missing != "tester.one" && e.Missing != "tester.two" {
			if got.State.List[i] != e {
				t.Fatalf("unselected position %d changed", i)
			}
			continue
		}
		m := got.State.Ix.ByID[got.State.List[i].Instance]
		if m == nil || m.PackageID != e.Missing {
			t.Fatalf("position %d: %+v", i, got.State.List[i])
		}
	}
	// A fresh read must include deployment candidates from the newly stored bindings.
	after, err := e.App.Loadout()
	if err != nil || !after.List.Equal(got.State.List) {
		t.Fatal("bindings did not survive reload", err)
	}
	if _, _, err := e.App.UndoRedo(false); err != nil {
		t.Fatal(err)
	}
	restored, _ := e.App.Loadout()
	if !restored.List.Equal(before.List) || e.App.Store.LastStepID(before.Plan.ID, "undo") != head {
		t.Fatal("batch was not one undo step")
	}
	if _, _, err := e.App.UndoRedo(true); err != nil {
		t.Fatal(err)
	}
	redone, _ := e.App.Loadout()
	if !redone.List.Equal(after.List) {
		t.Fatal("redo did not restore bindings")
	}
}

func TestUseInstalledRevalidatesBatch(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	dir := filepath.Join(e.SteamCMD, "one")
	e.WriteMod(dir, testkit.Sample("tester.one", "One"))
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.one", "tester.absent")
	scanNow(t, e)
	before, _ := e.App.Loadout()
	head := e.App.Store.LastStepID(before.Plan.ID, "undo")
	for _, packages := range [][]string{{"tester.one", "tester.absent"}, {"tester.not-in-list"}, {""}} {
		if _, err := e.App.UseInstalled(packages); err == nil {
			t.Fatalf("accepted %v", packages)
		}
		st, _ := e.App.Loadout()
		if !st.List.Equal(before.List) || e.App.Store.LastStepID(st.Plan.ID, "undo") != head {
			t.Fatal("failed batch changed workspace/history")
		}
	}
	if r, err := e.App.UseInstalled(nil); err != nil || len(r.Changes) != 0 {
		t.Fatal("empty batch", r, err)
	}
	// The offered copy becomes unloadable between rendering and clicking.
	e.Move(filepath.Join(dir, "About", "About.xml"), filepath.Join(dir, "About", "About.xml.off"))
	scanNow(t, e)
	if _, err := e.App.UseInstalled([]string{"tester.one"}); err == nil {
		t.Fatal("accepted stale unloadable copy")
	}
	st, _ := e.App.Loadout()
	if !st.List.Equal(before.List) || e.App.Store.LastStepID(st.Plan.ID, "undo") != head {
		t.Fatal("stale action changed workspace/history")
	}
}

func TestUseInstalledOtherGamesMods(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.foreign")
	scanNow(t, e)
	g2 := secondGame(e, "OtherGame")
	e.WriteMod(filepath.Join(g2, "Mods", "Foreign"), testkit.Sample("tester.foreign", "Foreign"))
	e.MustRun("env", "add", "other", "--game", g2, "--saves", "own")
	// Select the scan scope without EnvUse, which would create environment links.
	e.App.EnvName = "other"
	scanNow(t, e)
	e.App.EnvName = "main"
	st, _ := e.App.Loadout()
	missing := e.App.MissingMods(st)
	if len(missing) != 1 || !missing[0].CanEnable {
		t.Fatal(missing)
	}
	// The editor's row for the entry shows the other game's copy's name.
	if m := e.App.MissingByToken(st)["tester.foreign"]; m.Name != "Foreign" || !m.CanEnable {
		t.Fatalf("missing row: %+v", m)
	}
	m := st.Ix.ByID[missing[0].Instance]
	if m.Kind != source.Elsewhere || st.Reg.Loadable(m.ID) || !st.Deployable(m) {
		t.Fatalf("expected undeployed foreign copy: %+v", m)
	}
	before := st.List.Clone()
	r, err := e.App.UseInstalled([]string{"tester.foreign"})
	if err != nil || r.State.List[1].Instance != m.ID {
		t.Fatal(r, err)
	}
	// Only save, not the binding edit, deploys the link.
	link := filepath.Join(e.Game, "Mods", app.DeployName(m))
	if source.IsLink(link) {
		t.Fatal("binding deployed a link")
	}
	e.MustRun("save")
	if !source.IsLink(link) {
		t.Fatal("save did not deploy the bound foreign copy")
	}
	if _, _, err := e.App.UndoRedo(false); err != nil {
		t.Fatal(err)
	}
	// Save left a live link in the game, so Heal can now resolve the old token.
	// The stored undo payload must still be the exact pre-edit list.
	ws, _ := e.App.Store.Workspace(st.Plan.ID)
	if ws.Entries != before.Encode() {
		t.Fatal("undo lost original package entries")
	}
}

func TestUseInstalledDeduplicatesAndRejectsEnabledCopy(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	root := filepath.Join(e.Root, "Extra")
	e.WriteMod(filepath.Join(root, "one"), testkit.Sample("tester.one", "One"))
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.one", "TESTER.ONE_STEAM")
	e.AddEnv(false)
	if _, err := e.App.FolderAdd(source.Extra, root); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	r, err := e.App.UseInstalled([]string{"tester.one"})
	if err != nil || len(r.State.List) != 2 {
		t.Fatal(r, err)
	}
	st, _ := e.App.Loadout()
	// A leftover unresolved duplicate must never enable a second copy of a package.
	st.WS.Entries = append(st.List.Clone(), loadout.Entry{Missing: "tester.one"}).Encode()
	if err := e.App.Store.SetWorkspace(st.WS); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.UseInstalled([]string{"tester.one"}); err == nil {
		t.Fatal("accepted an already enabled package")
	}
}
