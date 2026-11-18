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

func TestDiscoverManagerImports(t *testing.T) {
	e := testkit.New(t)
	sort := filepath.Join(e.Root, "local", "RimSort")
	crow := filepath.Join(e.Root, "Programs", "RimCrow")
	config := filepath.Join(e.Root, "synthetic-user", "Config")
	settings, _ := json.Marshal(map[string]any{"instances": map[string]any{"Default": map[string]string{"config_folder": config}}})
	e.WriteFile(filepath.Join(sort, "settings.json"), string(settings))
	e.WriteFile(filepath.Join(sort, "dbs", "userRules.json"), `{"rules":{}}`)
	e.WriteFile(filepath.Join(sort, "modlists", "colony.xml"), "synthetic list")
	e.WriteFile(filepath.Join(config, "ModsConfig.xml"), "synthetic current list")
	e.WriteFile(filepath.Join(crow, "data", "mod_manager.db"), "synthetic database")
	e.WriteFile(filepath.Join(crow, "backups", "Latest_ModList.rml"), "synthetic saved list")
	// RimPy's saved lists: a synthetic ModsConfig-style XML in LocalLow\RimPy Mod Manager\ModLists
	rimpy := filepath.Join(e.Root, "LocalLow", "RimPy Mod Manager")
	rimpyList := filepath.Join(rimpy, "ModLists", "colony.xml")
	e.WriteFile(rimpyList, `<?xml version="1.0" encoding="utf-8"?><ModsConfigData><version>1.6.4633 rev1261</version><activeMods><li>ludeon.rimworld</li><li>tester.alpha</li></activeMods></ModsConfigData>`)
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
