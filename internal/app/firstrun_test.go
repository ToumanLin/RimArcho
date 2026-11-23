package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// syntheticList is a ModsConfig-style list with two Mods.
const syntheticList = `<?xml version="1.0" encoding="utf-8"?><ModsConfigData><version>1.6.4633 rev1261</version><activeMods><li>ludeon.rimworld</li><li>tester.alpha</li></activeMods></ModsConfigData>`

func TestDiscoverManagerImports(t *testing.T) {
	e := testkit.New(t)
	sort := filepath.Join(e.Root, "local", "RimSort")
	crow := filepath.Join(e.Root, "Programs", "RimCrow")
	config := filepath.Join(e.Root, "synthetic-user", "Config")
	settings, _ := json.Marshal(map[string]any{"instances": map[string]any{"Default": map[string]string{"config_folder": config}}})
	e.WriteFile(filepath.Join(sort, "settings.json"), string(settings))
	e.WriteFile(filepath.Join(sort, "dbs", "userRules.json"), `{"rules":{}}`)
	e.WriteFile(filepath.Join(sort, "modlists", "colony.xml"), syntheticList)
	e.WriteFile(filepath.Join(sort, "modlists", "notes.txt"), "not a list: an unrelated file beside the lists")
	e.WriteFile(filepath.Join(config, "ModsConfig.xml"), syntheticList)
	e.WriteFile(filepath.Join(crow, "data", "mod_manager.db"), "synthetic database")
	e.WriteFile(filepath.Join(crow, "backups", "Latest_ModList.rml"), syntheticList)
	e.WriteFile(filepath.Join(crow, "backups", "settings.json"), `{"theme":"dark"}`)
	// RimPy's saved lists: a synthetic ModsConfig-style XML in LocalLow\RimPy Mod Manager\ModLists
	rimpy := filepath.Join(e.Root, "LocalLow", "RimPy Mod Manager")
	rimpyList := filepath.Join(rimpy, "ModLists", "colony.xml")
	e.WriteFile(rimpyList, syntheticList)
	e.WriteFile(filepath.Join(rimpy, "config.ini"), "synthetic settings")
	before, _ := os.ReadFile(filepath.Join(sort, "settings.json"))
	found := app.DiscoverManagerImports(app.ManagerLocations{RimSort: []string{sort, sort}, Installs: []string{crow, filepath.Join(e.Root, "absent")}, RimPy: []string{rimpy}})
	if len(found) != 6 {
		t.Fatalf("candidates: %+v", found)
	}
	rimpyFound := false
	for _, f := range found {
		rimpyFound = rimpyFound || f.Manager == "RimPy" && f.Kind == "list" && f.Path == rimpyList
	}
	if !rimpyFound {
		t.Fatalf("RimPy list not found: %+v", found)
	}
	if l, err := app.ReadLoadoutFile(rimpyList); err != nil || len(l.Tokens) != 2 || l.Tokens[1] != "tester.alpha" {
		t.Fatalf("RimPy list: %+v %v", l, err)
	}
	kinds := map[string]int{}
	for _, f := range found {
		kinds[f.Kind]++
	}
	if kinds["list"] != 4 || kinds["rimcrow"] != 1 || kinds["rimsort-rules"] != 1 {
		t.Fatal(kinds)
	}
	after, _ := os.ReadFile(filepath.Join(sort, "settings.json"))
	if string(after) != string(before) {
		t.Fatal("discovery wrote settings")
	}
}

// A second home (RIMARCHO_HOME) on a Windows user whose default local folder another home
// already uses: before it has a Game Config it chooses its own local.dir (02 §2); once it
// has one, the default's contents may be its own and the choice is refused (VM 2026-11-23).
func TestManagedRootOfAHomeBesideAnother(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.App.LocalDir(), "monitor", "ArchoMonitor-0.12.0", "About", "About.xml"), "<ModMetaData/>")
	own := filepath.Join(e.Root, "second", "RimArcho")
	if err := e.App.SetManagedRoot(own); err != nil {
		t.Fatalf("a new home could not choose its own folder: %v", err)
	}
	if e.App.LocalDir() != own {
		t.Fatal(e.App.LocalDir())
	}

	used := testkit.New(t)
	used.WriteFile(filepath.Join(used.App.LocalDir(), "monitor", "ArchoMonitor-0.12.0", "About", "About.xml"), "<ModMetaData/>")
	used.AddEnv(false)
	if err := used.App.SetManagedRoot(filepath.Join(used.Root, "another")); err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("a home with a Game Config abandoned its default folder: %v", err)
	}
}

func TestManagedRootAndWorkshopRefusal(t *testing.T) {
	e := testkit.New(t)
	root := filepath.Join(e.Root, "chosen", "RimArcho")
	if err := e.App.SetManagedRoot(e.Home); err == nil {
		t.Fatal("accepted data-directory overlap")
	}
	if err := e.App.SetManagedRoot(root); err != nil {
		t.Fatal(err)
	}
	if e.App.SteamCMDDir() != filepath.Join(root, "steamcmd") || e.App.LockDir() != filepath.Join(root, "locked") || e.App.ToddsDir() != filepath.Join(root, "todds") {
		t.Fatal("managed defaults did not follow root")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("selection created a folder")
	}
	e.WriteFile(filepath.Join(root, "steamcmd", deps.SteamCMDExecutable()), "synthetic")
	if err := e.App.SetManagedRoot(filepath.Join(e.Root, "another")); err == nil {
		t.Fatal("silently abandoned used root")
	}
	e.App.SelfProgram = filepath.Join(e.Workshop, "123", "extracted")
	refusal := "Workshop folder"
	if !app.SelfUpdateSupported() {
		refusal = "update RimArcho manually"
	}
	if _, err := e.App.InstallSelfUpdate(context.Background(), nil); err == nil || !strings.Contains(err.Error(), refusal) {
		t.Fatalf("update refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.App.SelfProgram, app.UpdateDirName)); !os.IsNotExist(err) {
		t.Fatal("refused update wrote files")
	}
}

// 07 §6.2: a portable RimCrow anywhere is found through its `_Link_` links in a game's Mods
// folder (they point into <install>\mods), and a chosen .exe or folder reads like a found
// install; a link into another folder, or an .exe with no data beside it, finds nothing.
func TestDiscoverPortableManagers(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	crow := filepath.Join(e.Root, "D", "Game", "RimModManager")
	e.WriteFile(filepath.Join(crow, "RimModManager.exe"), "MZ")
	e.WriteFile(filepath.Join(crow, "data", "mod_manager.db"), "synthetic database")
	e.WriteFile(filepath.Join(crow, "mods", "123", "About", "About.xml"), "<ModMetaData/>")
	other := filepath.Join(e.Root, "elsewhere", "456")
	e.WriteFile(filepath.Join(other, "About", "About.xml"), "<ModMetaData/>")
	mods := filepath.Join(e.Root, "game", "Mods")
	e.Junction(filepath.Join(mods, "_Link_123"), filepath.Join(crow, "mods", "123"))
	e.Junction(filepath.Join(mods, "_Link_456"), other)
	db := filepath.Join(crow, "data", "mod_manager.db")
	found := app.DiscoverManagerImports(app.ManagerLocations{GameMods: []string{mods}})
	if len(found) != 1 || found[0].Kind != "rimcrow" || !strings.EqualFold(found[0].Path, db) {
		t.Fatalf("through the link: %+v", found)
	}
	for _, pick := range []string{filepath.Join(crow, "RimModManager.exe"), crow} {
		if got := app.ManagerSourcesIn(pick); len(got) != 1 || got[0].Kind != "rimcrow" || !strings.EqualFold(got[0].Path, db) {
			t.Fatalf("%s: %+v", pick, got)
		}
	}
	lone := filepath.Join(e.Root, "Tools", "RimSort.exe")
	e.WriteFile(lone, "MZ")
	if got := app.ManagerSourcesIn(lone); len(got) != 0 {
		t.Fatalf("an .exe with no data: %+v", got)
	}
	if got := app.ManagerSourcesIn(db); got != nil {
		t.Fatalf("a data file is classified by ManagerFileKind, not read as a folder: %+v", got)
	}
}
