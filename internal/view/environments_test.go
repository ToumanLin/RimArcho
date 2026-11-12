package view_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestPhase3EnvironmentViews(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("env", "add", "sibling", "--like", "main")
	other := e.OtherGame("second", "1.6.4633 rev1261")
	e.MustRun("env", "add", "another", "--game", other, "--saves", "own")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	page := view.BuildEnvPage(e.App, st)
	if len(page.Envs) != 3 || page.Envs[0].Name != "another" || page.Envs[1].Name != "main" || page.Envs[2].Name != "sibling" {
		t.Fatalf("group order: %+v", page.Envs)
	}
	if page.Envs[1].GameID != page.Envs[2].GameID || page.Envs[1].Installation != page.Envs[2].Installation || !page.Envs[1].Current {
		t.Fatalf("groups/current: %+v", page.Envs)
	}
	if len(page.Groups) != 1 || page.Groups[0].Title != "Game Config" || len(page.Groups[0].Items) != 7 {
		t.Fatalf("settings: %+v", page.Groups)
	}
	preview, err := e.App.EnvRemovePreview("main")
	if err != nil {
		t.Fatal(err)
	}
	if preview.LastSibling || !strings.Contains(strings.Join(view.EnvRemovalLines(preview), " "), "inventory and analysis stay") {
		t.Fatalf("sibling preview: %+v", preview)
	}
	last, err := e.App.EnvRemovePreview("another")
	if err != nil {
		t.Fatal(err)
	}
	if !last.LastSibling || !strings.Contains(strings.Join(view.EnvRemovalLines(last), " "), "last Game Config") {
		t.Fatalf("last preview: %+v", last)
	}
	move, err := e.App.EnvGameChangePreview("main", e.Root+"/relocated")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(move, " "), "1 other Game Configs use this folder and will change too: sibling") {
		t.Fatal(move)
	}
	reattach, err := e.App.EnvGameChangePreview("main", other)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(reattach, " "), "only Game Config main") {
		t.Fatal(reattach)
	}
	data, _ := json.MarshalIndent(page, "", "  ")
	t.Log(strings.ReplaceAll(string(data), e.Root, "<fixture>"))
	t.Log(view.EnvRemovalLines(preview))
	t.Log(view.EnvRemovalLines(last))
	t.Log(move)
	t.Log(reattach)
}

func TestLegacyAliasNoticeAndProtonAdd(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	other := e.OtherGame("old-alias", "1.6.4633 rev1261")
	e.MustRun("env", "add", "legacy", "--game", other, "--saves", "own")
	legacy, _ := e.App.Store.EnvByName("legacy")
	// Pre-upgrade root keys remain distinct even when paths later resolve to the same installation.
	if _, err := e.App.Store.DB.Exec("UPDATE game SET game_root=? WHERE id=?", e.Game, legacy.GameID); err != nil {
		t.Fatal(err)
	}
	page := view.BuildEnvPage(e.App, nil)
	if len(page.Notices) != 1 || !strings.Contains(page.Notices[0], "legacy, main") {
		t.Fatalf("notices: %+v", page.Notices)
	}
	if err := e.App.Store.SetMeta("notice.shared-game-folder", "seen"); err != nil {
		t.Fatal(err)
	}
	if again := view.BuildEnvPage(e.App, nil); len(again.Notices) != 0 {
		t.Fatal("alias notice repeated")
	}
	es, _ := e.App.Store.Envs()
	if len(es) != 2 || es[0].GameID == es[1].GameID {
		t.Fatal("alias notice merged records")
	}
	if err := os.MkdirAll(filepath.Join(e.Steam, "steamapps", "compatdata", "294100", "pfx"), 0755); err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64.exe"), "synthetic")
	e.App.Detect.GOOS = "linux"
	for _, row := range view.EnvironmentRows(e.App, "") {
		if row.AddReason == "" {
			t.Fatalf("Proton add enabled: %+v", row)
		}
	}
}
