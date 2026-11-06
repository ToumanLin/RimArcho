package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
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
	before, _ := os.ReadFile(filepath.Join(sort, "settings.json"))
	found := app.DiscoverManagerImports(app.ManagerLocations{RimSort: []string{sort, sort}, Installs: []string{crow, filepath.Join(e.Root, "absent")}})
	if len(found) != 5 {
		t.Fatalf("candidates: %+v", found)
	}
	kinds := map[string]int{}
	for _, f := range found {
		kinds[f.Kind]++
	}
	if kinds["list"] != 3 || kinds["rimcrow"] != 1 || kinds["rimsort-rules"] != 1 {
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
	e.WriteFile(filepath.Join(root, "steamcmd", "steamcmd.exe"), "synthetic")
	if err := e.App.SetManagedRoot(filepath.Join(e.Root, "another")); err == nil {
		t.Fatal("silently abandoned used root")
	}
	e.App.SelfProgram = filepath.Join(e.Workshop, "123", "extracted")
	if _, err := e.App.InstallSelfUpdate(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "Workshop folder") {
		t.Fatalf("update refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.App.SelfProgram, app.UpdateDirName)); !os.IsNotExist(err) {
		t.Fatal("refused update wrote files")
	}
}
