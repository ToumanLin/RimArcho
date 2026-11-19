package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func foldersChanged(t *testing.T, e *testkit.Env) bool {
	t.Helper()
	ok, err := e.App.ModFoldersChanged()
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

// GitHub #10 part b: the player subscribes in Steam and enables the new mods in the game's
// menu while RimArcho is not looking. The outside change loads the list with the new mods
// as missing entries (the library has not read them); the folder check notices the change,
// and the rescan it starts heals them in place (04 §1.6, 07 §4.2).
func TestSubscribedOutsideHealsAfterFolderCheck(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	if foldersChanged(t, e) {
		t.Fatal("before the first scan the check must stay quiet (the guide scans)")
	}
	scanNow(t, e)
	if foldersChanged(t, e) {
		t.Fatal("changed right after a scan")
	}
	e.MustRun("status") // the list is imported and seen

	// Steam downloads two new subscriptions; the game's menu enables them.
	e.AddWorkshop("3001", testkit.Sample("tester.chess", "Chess"))
	e.AddWorkshop("3002", testkit.Sample("tester.seed", "Seed"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.a", "tester.chess", "tester.seed")
	if got := joined(active(t, e)); got != "Core, Harmony, A, ?tester.chess, ?tester.seed" {
		t.Fatalf("before the rescan: %s", got)
	}
	if !foldersChanged(t, e) {
		t.Fatal("new Workshop folders not noticed")
	}
	scanNow(t, e)
	if got := joined(active(t, e)); got != "Core, Harmony, A, Chess, Seed" {
		t.Fatalf("after the rescan the entries did not heal: %s", got)
	}
	if foldersChanged(t, e) {
		t.Fatal("still changed after the rescan")
	}

	// A local mod copied into Mods/ and one removed from it are changes too.
	e.WriteMod(filepath.Join(e.Game, "Mods", "C"), testkit.Sample("tester.c", "C"))
	if !foldersChanged(t, e) {
		t.Fatal("a new Mods/ folder not noticed")
	}
	scanNow(t, e)
	e.Remove(filepath.Join(e.Game, "Mods", "C"))
	if !foldersChanged(t, e) {
		t.Fatal("a removed Mods/ folder not noticed")
	}
	scanNow(t, e)

	// Steam rewrites its manifest (an update, an unsubscribe): a change.
	e.Unsubscribe("3002")
	if !foldersChanged(t, e) {
		t.Fatal("a rewritten Steam manifest not noticed")
	}
}

// A subscription Steam has not finished is "still downloading", not "not installed": the
// manifest lists it, or Steam's downloads folder has it, and the content folder has no
// About.xml yet. Once the folder is complete the next scan heals the entry.
func TestWorkshopPendingDownload(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	// steamDB knows the new item's packageId (the missing entry's Workshop ID).
	e.WriteFile(filepath.Join(e.App.Home, "cache", "steamDB.index.json"),
		`{"tester.chess":["3001","Chess"],"tester.seed":["3002","Seed"],"tester.gone":["3003","Gone"]}`)
	e.Subscribe("3001")                                                                        // listed, no folder yet
	e.MkdirAll(filepath.Join(e.Steam, "steamapps", "workshop", "downloads", "294100", "3002")) // in Steam's downloads folder
	e.MkdirAll(filepath.Join(e.Workshop, "3002"))                                              // an empty content folder so far
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.a", "tester.chess", "tester.seed", "tester.gone")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if p := e.App.WorkshopPending(); !p["3001"] || !p["3002"] || p["3003"] || p["1001"] || len(p) != 2 {
		t.Fatalf("pending: %v", p)
	}
	got := map[string]bool{}
	for _, m := range e.App.MissingMods(st) {
		got[m.PackageID] = m.Downloading
	}
	if !got["tester.chess"] || !got["tester.seed"] || got["tester.gone"] {
		t.Fatalf("downloading flags: %v", got)
	}

	// Steam finishes: the folder is complete and the downloads folder is gone.
	e.WriteMod(filepath.Join(e.Workshop, "3001"), testkit.Sample("tester.chess", "Chess"))
	if err := os.RemoveAll(filepath.Join(e.Steam, "steamapps", "workshop", "downloads", "294100", "3002")); err != nil {
		t.Fatal(err)
	}
	e.WriteMod(filepath.Join(e.Workshop, "3002"), testkit.Sample("tester.seed", "Seed"))
	e.Subscribe("3002")
	if !foldersChanged(t, e) {
		t.Fatal("finished downloads not noticed")
	}
	scanNow(t, e)
	if p := e.App.WorkshopPending(); len(p) != 0 {
		t.Fatalf("still pending: %v", p)
	}
	if got := joined(active(t, e)); got != "Core, Harmony, A, Chess, Seed, ?tester.gone" {
		t.Fatalf("after Steam finished: %s", got)
	}
}
