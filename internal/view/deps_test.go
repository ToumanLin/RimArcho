package view_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestExternalToolsSettings(t *testing.T) {
	e, _, _ := shellEnv(t)
	e.App.SteamCMDNoPath = true
	path := filepath.Join(e.App.LocalDir(), deps.SteamCMD, "steamcmd.exe")
	e.WriteFile(path, "fake")
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		t.Cleanup(func() { view.SetLang("en") })
		groups := view.SettingsPage(e.App, nil)
		found := false
		for _, g := range groups {
			if g.Title == view.T("texture optimisation") {
				t.Fatal("old todds group remains")
			}
			if g.Title == "SteamCMD" {
				if len(g.Items) != 1 || g.Items[0].Argv[2] != "steamcmd.account" {
					t.Fatalf("SteamCMD account group %+v", g)
				}
			}
			if g.Title != view.T("external tools") {
				continue
			}
			found = true
			if lang == "zh" && g.Title != "外部工具" {
				t.Fatal(g.Title)
			}
			if len(g.Items) != len(deps.Names) {
				t.Fatalf("rows %+v", g.Items)
			}
			for j, it := range g.Items {
				name := deps.Names[j]
				if name == deps.Font {
					if it.Edit != "" || len(it.Actions) != 0 || it.Value == "" {
						t.Fatalf("font controls %+v", it)
					}
					continue
				}
				if it.Edit != "path" || it.EditValue == nil || !it.File || view.SettingCommand(it.Argv) != nil {
					t.Fatalf("path editor %+v", it)
				}
				if it.Value == "" || len(it.Actions) == 0 {
					t.Fatalf("tool status/actions %+v", it)
				}
				if name == deps.SteamCMD {
					if it.Value != view.T("installed") || !strings.Contains(it.Note, path) || len(it.Actions) != 2 || *it.EditValue != "" {
						t.Fatalf("own SteamCMD %+v", it)
					}
				}
				if name == deps.WezTerm && !strings.Contains(it.Note, view.T("only needed for the terminal interface (archo tui)")) {
					t.Fatal(it.Note)
				}
				for _, act := range it.Actions {
					if view.SettingCommand(act.Argv) != nil || act.Task == "" || act.Pending == "" || act.Confirm == "" || !view.IsSettingTask(act.Task) {
						t.Fatalf("action %+v", act)
					}
				}
			}
		}
		if !found {
			t.Fatal("missing external tools group")
		}
	}
}

func TestExternalToolConfiguredEditValue(t *testing.T) {
	e, _, _ := shellEnv(t)
	external := filepath.Join(e.Root, "external", "steamcmd.exe")
	e.WriteFile(external, "external")
	e.MustRun("config", "set", "steamcmd.path", external)
	g := view.DependencyGroup(e.App)
	it := g.Items[2]
	if it.EditValue == nil || *it.EditValue != external || len(it.Actions) != 1 || !strings.Contains(it.Note, external) {
		t.Fatalf("configured row %+v", it)
	}
}

func TestSteamCMDRemovalHiddenWhenItHoldsMods(t *testing.T) {
	e, _, _ := shellEnv(t)
	e.App.SteamCMDNoPath = true
	dir := filepath.Join(e.App.LocalDir(), deps.SteamCMD)
	e.WriteFile(filepath.Join(dir, "steamcmd.exe"), "tool")
	e.WriteFile(filepath.Join(dir, "steamapps", "workshop", "content", "294100", "123", "keep.txt"), "mod")
	it := view.DependencyGroup(e.App).Items[2]
	if len(it.Actions) != 1 || it.Actions[0].Argv[1] != "install" {
		t.Fatalf("unsafe removal offered: %+v", it.Actions)
	}
}
