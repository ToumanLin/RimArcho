package view_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The first run's "found" step (16 §2.15): each game with its Mod folders counted, the
// folders another manager uses that are not the game's or Steam's own, and where detection
// looked. A ticked manager folder becomes a shared Mod folder when the game is added.
func TestFirstRunFoundFolders(t *testing.T) {
	e := testkit.New(t)
	e.WriteMod(filepath.Join(e.Workshop, "1001"), testkit.Sample("a.one", "One"))
	e.WriteMod(filepath.Join(e.Workshop, "1002"), testkit.Sample("a.two", "Two"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Local"), testkit.Sample("a.local", "Local"))
	dmods := filepath.Join(e.Root, "D-Mods")
	e.WriteMod(filepath.Join(dmods, "ExtraPack"), testkit.Sample("a.extra", "Extra"))
	empty := filepath.Join(e.Root, "Empty")
	e.MkdirAll(empty)
	managers := []app.ManagerFolder{
		{Manager: "RimSort", Path: dmods},
		{Manager: "RimSort", Path: filepath.Join(e.Game, "Mods")}, // the game's own: shown as the game's row
		{Manager: "RimSort", Path: e.Workshop},                    // Steam's: shown as the Workshop row
		{Manager: "RimSort", Path: empty},                         // no Mods: nothing to offer
	}
	f := view.FirstRun(e.App, managers)
	if len(f.Games) != 1 || f.Libraries != 1 || len(f.Folders) != 0 {
		t.Fatalf("found %+v", f)
	}
	g := f.Games[0]
	if g.Game != e.Game || g.Workshop != e.Workshop || g.WorkshopMods != 2 || g.Mods != 1 {
		t.Fatalf("game %+v", g)
	}
	if len(f.Managers) != 1 || f.Managers[0].Path != dmods || f.Managers[0].Mods != 1 || f.Managers[0].Manager != "RimSort" {
		t.Fatalf("managers %+v", f.Managers)
	}

	g.Extra = []string{dmods}
	if _, err := view.AddGames(e.App, []view.GameFound{g}); err != nil {
		t.Fatal(err)
	}
	srcs, err := e.App.Store.SharedSources()
	if err != nil {
		t.Fatal(err)
	}
	var extra, workshop bool
	for _, s := range srcs {
		extra = extra || s.Kind == source.Extra && s.Root == dmods
		workshop = workshop || s.Kind == source.Workshop && s.Root == e.Workshop
	}
	if !extra || !workshop {
		t.Fatalf("shared folders %+v", srcs)
	}
}

// "no RimWorld found" says where detection looked: the Steam libraries and the games
// folders that exist.
func TestFirstRunNothingFound(t *testing.T) {
	e := testkit.New(t)
	e.Remove(e.Game)
	games := filepath.Join(e.Root, "Games")
	e.MkdirAll(games)
	e.App.Detect.ManualRoots = []string{filepath.Join(games, "RimWorld")}
	f := view.FirstRun(e.App, nil)
	if len(f.Games) != 0 || f.Libraries != 1 || len(f.Folders) != 1 || f.Folders[0] != games || f.Managers == nil {
		t.Fatalf("found %+v", f)
	}
}

// The TUI's empty screen (16 §9): the detected games no Game Config uses; a later version
// on the same user data folder gets its own saves, also beside a game added already, the guide's default (16 §2.15).
func TestDetectedNewOwnSaves(t *testing.T) {
	e := testkit.New(t)
	g17 := e.OtherGame("RimWorld 1.7", "1.7.5014 rev1300")
	e.App.Detect.ManualRoots = []string{g17}
	games := view.DetectedNew(e.App)
	if len(games) != 2 || games[0].Game != e.Game || games[1].Game != g17 {
		t.Fatalf("games %+v", games)
	}
	if games[0].Saves != "" || games[1].Saves != app.SavesOwn || !games[1].GameList {
		t.Fatalf("saves %q %q list %v", games[0].Saves, games[1].Saves, games[1].GameList)
	}
	if _, err := view.AddGames(e.App, games[:1]); err != nil {
		t.Fatal(err)
	}
	if left := view.DetectedNew(e.App); len(left) != 1 || left[0].Game != g17 || left[0].Saves != app.SavesOwn {
		t.Fatalf("after adding 1.6: %+v", left)
	}
}
