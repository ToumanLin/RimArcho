package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// 06 §2.14: the settings page has the game log group with the monitor.default switch.
func TestSettingsMonitor(t *testing.T) {
	e, _, _ := shellEnv(t)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range view.SettingsPage(e.App, st) {
		if g.Title != "game log" {
			continue
		}
		it := g.Items[0]
		if it.Value != "on" || strings.Join(it.Argv, " ") != "config set monitor.default {}" || view.SettingCommand([]string{"config", "set", "monitor.default", "off"}) != nil {
			t.Fatalf("item %+v", it)
		}
		if len(g.Items) < 2 || g.Items[1].Value != "not installed" {
			t.Fatalf("installed item %+v", g.Items)
		}
		return
	}
	t.Fatal("no game log group")
}

// GPU and Monitor help is available on hover; only the restart note sits by the value.
func TestSettingsWording(t *testing.T) {
	e, _, _ := shellEnv(t)
	e.MustRun("config", "set", "ui.gpu", "off")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { view.SetLang("en") })
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		found := 0
		for _, group := range view.SettingsPage(e.App, st) {
			for _, it := range group.Items {
				if len(it.Argv) < 3 {
					continue
				}
				switch it.Argv[2] {
				case "ui.gpu":
					found++
					if it.Note != "" || it.Hover != view.T("uses about 20–35 MB less memory and draws in software") || it.ValueNote != view.T("takes effect next time you open RimArcho") {
						t.Errorf("GPU help: %+v", it)
					}
				case "monitor.default":
					found++
					if it.Note != "" || it.Hover != view.T("new lists, and every sort") {
						t.Errorf("Monitor help: %+v", it)
					}
				}
			}
		}
		if found != 2 {
			t.Fatalf("found %d settings", found)
		}
	}
}
