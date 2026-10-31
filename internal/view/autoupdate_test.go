package view_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestAutomaticSettingsOnlyControlsAndHumanIntervals(t *testing.T) {
	defer view.SetLang("en")
	e, _, _ := shellEnv(t)
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		for _, preset := range []struct {
			value  string
			en, zh [3]string
		}{
			{"normal", [3]string{"daily", "weekly", "every 4 hours"}, [3]string{"每天", "每周", "每 4 小时"}},
			{"frequent", [3]string{"every 12 hours", "twice a week", "every 2 hours"}, [3]string{"每 12 小时", "每周两次", "每 2 小时"}},
			{"relaxed", [3]string{"every 2 days", "every 2 weeks", "every 8 hours"}, [3]string{"每 2 天", "每 2 周", "每 8 小时"}},
		} {
			intervals := preset.en
			if lang == "zh" {
				intervals = preset.zh
			}
			e.MustRun("config", "set", "updates.interval", preset.value)
			g := view.AutomaticGroup(e.App)
			// RimArcho's own version (07 §5.1), then the two controls
			if len(g.Items) != 3 || g.Items[0].Label != "RimArcho" || strings.Contains(g.Note, "LoversLab") {
				t.Fatal(g)
			}
			want := view.Tf("While RimArcho is open: check mods, community data and RimArcho %s, tools %s.", intervals[0], intervals[1]) + " " + view.T("Mod, tool and RimArcho updates wait for your click.")
			if g.Note != want {
				t.Fatalf("%s/%s: %s, want %s", lang, preset.value, g.Note, want)
			}
			if err := e.App.Store.SetMeta("account:loverslab:enabled", "true"); err != nil {
				t.Fatal(err)
			}
			if note := view.AutomaticGroup(e.App).Note; !strings.Contains(note, view.Tf("Check the LoversLab login %s.", intervals[2])) {
				t.Fatal(note)
			}
			if err := e.App.Store.SetMeta("account:loverslab:enabled", "false"); err != nil {
				t.Fatal(err)
			}
		}
	}
	view.SetLang("zh")
	if got := view.AutomaticGroup(e.App).Items[1].ChoiceLabels; !reflect.DeepEqual(got, []string{"开", "关"}) {
		t.Fatal(got)
	}
	for _, g := range view.SettingsPage(e.App, nil) {
		for _, it := range g.Items {
			if reflect.DeepEqual(it.Choices, []string{"on", "off"}) && !reflect.DeepEqual(it.ChoiceLabels, []string{"开", "关"}) {
				t.Fatal(it)
			}
		}
	}
}

func TestAutomaticDatesBelongToTheirSubjects(t *testing.T) {
	defer view.SetLang("en")
	e, _, _ := shellEnv(t)
	e.App.SteamCMDNoPath = true
	e.WriteFile(filepath.Join(e.App.LocalDir(), deps.SteamCMD, "steamcmd.exe"), "synthetic tool")
	seed := func(key string, value any) {
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.App.Store.SetMeta(key, string(b)); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range app.AutomaticKinds {
		seed("automatic:"+kind, app.AutomaticStatus{Kind: kind, Checked: e.Now(), Completed: e.Now(), Pending: 1})
	}
	seed("tool-update:"+deps.SteamCMD, app.ToolUpdate{Name: deps.SteamCMD, Checked: e.Now(), Latest: "2345", Pending: true, Compatible: true})
	for _, lang := range []string{"zh", "en"} {
		view.SetLang(lang)
		date := view.Tf("last checked %s", e.App.FormatTime(e.Now()))
		for _, g := range view.SettingsPage(e.App, nil) {
			switch g.Title {
			case view.T("automatic checks"):
				if strings.Contains(g.Note, date) || len(g.Items) != 3 {
					t.Fatal(g)
				}
			case view.T("external tools"), view.T("community data"):
				if g.Note != date {
					t.Fatal(g)
				}
			}
		}
		found := false
		for _, it := range view.DependencyGroup(e.App).Items {
			if it.Label == "SteamCMD" {
				found = true
				if it.ValueNote != view.Tf("new version available: %s", "2345") {
					t.Fatal(it)
				}
			}
		}
		if !found {
			t.Fatal("missing SteamCMD")
		}
		if line, signedIn := view.LoversLabLine(e.App); line != "" || signedIn || view.AccountGroup(e.App).Items[1].Note != "" {
			t.Fatal("unconfigured account status", line, signedIn)
		}
		st, err := e.App.Loadout()
		if err != nil {
			t.Fatal(err)
		}
		p, err := view.BuildSubscriptions(e.App, st, modchanges.Opts{})
		if err != nil || p.Sources[3].Account != "" {
			t.Fatal(p, err)
		}
		if err := e.App.Store.SetMeta("account:loverslab:enabled", "true"); err != nil {
			t.Fatal(err)
		}
		if note := view.AccountGroup(e.App).Items[1].Note; !strings.Contains(note, date) {
			t.Fatal(note)
		}
		if err := e.App.Store.SetMeta("account:loverslab:enabled", "false"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckResultsShowUpdates(t *testing.T) {
	defer view.SetLang("en")
	for _, lang := range []string{"zh", "en"} {
		view.SetLang(lang)
		for _, state := range []string{"check complete", "up to date"} {
			if got := view.AutomaticResult(app.AutomaticStatus{State: state, Pending: 1}); got != view.T("1 update available") {
				t.Fatal(got)
			}
			if got := view.AutomaticResult(app.AutomaticStatus{State: state, Pending: 3}); got != view.Tf("%d updates available", 3) {
				t.Fatal(got)
			}
			if got := view.AutomaticResult(app.AutomaticStatus{State: state}); got != view.T("everything is up to date") {
				t.Fatal(got)
			}
		}
		if got := view.AutomaticResult(app.AutomaticStatus{State: "check unavailable offline", Pending: 3}); got != view.T("check unavailable offline") {
			t.Fatal("failure hidden", got)
		}
		const path = `D:\synthetic\cache\backup`
		message := "community data rollback failed; backups kept at " + path + ": synthetic failure"
		want := message
		if lang == "zh" {
			want = "未能恢复社区数据，旧文件保留在 " + path + "：synthetic failure"
		}
		if got := view.Text(message); got != want {
			t.Fatal(got)
		}
	}
}
