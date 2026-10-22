package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// 06 §2.14: the settings page has the 游戏日志 group with the monitor.default switch.
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
