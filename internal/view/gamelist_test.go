package view_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Add Game Config's "the game's current list" (16 §2.15): the found game says how many Mods
// its ModsConfig.xml enables, and a config with its own saves folder starts from a copy of
// that file instead of the copied config's list.
func TestAddGameConfigStartsFromTheGamesList(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	var found *view.GameFound
	for _, g := range view.FindGames(e.App) {
		if g.Current {
			found = &g
		}
	}
	if found == nil || found.GameMods == 0 {
		t.Fatalf("the current game's ModsConfig.xml count: %+v", found)
	}
	one := *found
	one.Name, one.AddConfig, one.Saves, one.Like, one.GameList = "own-copy", true, app.SavesOwn, "main", true
	if _, err := view.CreateGames(e.App, []view.GameFound{one}); err != nil {
		t.Fatal(err)
	}
	ne, _ := e.App.Store.EnvByName("own-copy")
	if ne == nil {
		t.Fatal("no config added")
	}
	want, _ := os.ReadFile(env.ModsConfigPath(e.UserData))
	got, err := os.ReadFile(env.ModsConfigPath(ne.UserDataRoot))
	if err != nil || string(got) != string(want) {
		t.Fatalf("the game's ModsConfig.xml was not copied: %v", err)
	}
	if p, _ := e.App.Store.CurrentPlan(ne.ID); p != nil {
		t.Fatalf("the copied config's list was taken: %s", p.Name)
	}
}

// The Launch page's game-update line (16 §2.3): a new Version.txt since the scan, or a
// Steam manifest that flags an update; none otherwise.
func TestGameUpdateLine(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	ge, _ := e.App.Store.EnvByName("main")
	if l := view.GameUpdateHome(e.App, ge); l != nil {
		t.Fatalf("no update yet: %+v", l)
	}
	manifest := filepath.Join(filepath.Dir(filepath.Dir(e.Game)), "appmanifest_294100.acf")
	e.WriteFile(manifest, "\"AppState\"\n{\n\t\"appid\"\t\"294100\"\n\t\"StateFlags\"\t\"6\"\n\t\"buildid\"\t\"1\"\n}\n")
	if l := view.GameUpdateHome(e.App, ge); l == nil || l.Text != "Steam has an update waiting" || l.Link != "back up main first" {
		t.Fatalf("Steam update: %+v", l)
	}
	e.WriteFile(manifest, "\"AppState\"\n{\n\t\"StateFlags\"\t\"4\"\n\t\"buildid\"\t\"1\"\n}\n")
	if l := view.GameUpdateHome(e.App, ge); l != nil {
		t.Fatalf("installed and current: %+v", l)
	}
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.6.9999 rev1")
	if l := view.GameUpdateHome(e.App, ge); l == nil || l.Text != "updated to 1.6.9999 rev1 since the last scan" {
		t.Fatalf("version changed: %+v", l)
	}
}
