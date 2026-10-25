package view_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestCompactAccountRows(t *testing.T) {
	defer view.SetLang("en")
	e, _, _ := shellEnv(t)
	view.SetLang("zh")
	e.App.SteamCMDNoPath = true
	e.WriteFile(filepath.Join(e.App.LocalDir(), "steamcmd", "steamcmd.exe"), "offline fixture")
	group := view.AccountGroup(e.App)
	steam, ll := group.Items[0], group.Items[1]
	if steam.Note != "下载Mod可用，未登录" || ll.Note != "不可用，未登录" {
		t.Fatal(group)
	}
	for _, item := range group.Items {
		if item.Edit != "" || len(item.Actions) != 1 || item.Actions[0].Label != "登录" || strings.ContainsAny(item.Note, "\n；") {
			t.Fatal(item)
		}
	}
	if steam.Actions[0].Input == nil || ll.Actions[0].Confirm == "" {
		t.Fatal(group)
	}
	if err := e.App.Store.SetMeta("account:loverslab:consent", "true"); err != nil {
		t.Fatal(err)
	}
	if view.AccountGroup(e.App).Items[1].Actions[0].Confirm != "" {
		t.Fatal("consent repeated")
	}
	for _, owned := range []bool{false, true} {
		status := app.SteamAccountStatus{Account: "dummyPlayer", LoggedIn: true, OwnsRimWorld: owned, Managed: true}
		text := view.SteamAccountLine(status)
		expected := "下载Mod可用，下载游戏不可用，登录账号：dummyPlayer"
		if owned {
			expected = "下载Mod可用，下载游戏可用，登录账号：dummyPlayer"
		}
		if text != expected {
			t.Fatal(text)
		}
	}
	external := filepath.Join(e.Root, "external", "steamcmd.exe")
	e.WriteFile(external, "fixture")
	e.MustRun("config", "set", "steamcmd.path", external)
	steam = view.AccountGroup(e.App).Items[0]
	if steam.Note != "使用外部 SteamCMD，登录由它自己管理" || len(steam.Actions) != 0 {
		t.Fatal(steam)
	}
	b, _ := json.Marshal(steam)
	if strings.Contains(string(b), "edit") {
		t.Fatal(string(b))
	}
}

func TestAccountSubscriptionsAndTaskResultsEnglishHaveNoCJK(t *testing.T) {
	e, _, _ := shellEnv(t)
	view.SetLang("en")
	e.App.SteamCMDNoPath = true
	e.WriteFile(filepath.Join(e.App.LocalDir(), "steamcmd", "steamcmd.exe"), "offline")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	p, err := view.BuildSubscriptions(e.App, st, modchanges.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	tasks := view.BuildTasks([]app.Task{
		{ID: "1", Kind: view.TaskAccount, Lane: app.LaneSelf, State: app.TaskDone, Result: "玩家操作已完成"},
		{ID: "2", Kind: view.TaskSiteDownload, Lane: app.LaneSelf, State: app.TaskFailed, Result: "下载选择已失效，请重新读取"},
		{ID: "3", Kind: view.TaskSourceCheck, Lane: app.LaneSelf, State: app.TaskDone, Result: "check complete"},
	}, time.Now())
	data := []any{view.AccountGroup(e.App), p, tasks, view.LinkReason(&app.GameLink{State: source.TargetState("missing")}), view.Text("最近登录成功"), view.Text("environment create failed 0x80004005")}
	for _, value := range data {
		b, _ := json.Marshal(value)
		for _, r := range string(b) {
			if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
				t.Fatalf("CJK in English output: %s", b)
			}
		}
	}
	for _, owned := range []bool{false, true} {
		line := view.SteamAccountLine(app.SteamAccountStatus{Account: "dummyPlayer", Managed: true, LoggedIn: true, OwnsRimWorld: owned})
		for _, r := range line {
			if unicode.Is(unicode.Han, r) {
				t.Fatal(line)
			}
		}
	}
}
