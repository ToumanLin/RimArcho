package view

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func TestSubscriptionTaskTextAndActions(t *testing.T) {
	SetLang("zh")
	defer SetLang("en")
	task := app.Task{Kind: TaskSubscribe, Lane: app.LaneSelf, State: app.TaskPartial, Total: 5, Done: 5, Result: "raw repeated error https://example.invalid", Items: []app.TaskItem{
		{Ref: "ws:1", Name: "Actual Mod", State: app.ItemOK},
		{Ref: "ws:2", Name: "Deleted Mod", State: app.ItemFail, FailureKind: "missing", Permanent: true},
		{Ref: "ws:3", Name: "Private Mod", State: app.ItemFail, FailureKind: "missing", Permanent: true},
		{Ref: "ws:4", Name: "Network Mod", State: app.ItemFail, FailureKind: "network"},
		{Ref: "ws:5", Name: "Denied Mod", State: app.ItemFail, FailureKind: "permission", Permanent: true},
	}}
	p := BuildTasks([]app.Task{task}, time.Now())
	r := p.Rows[0]
	if len(p.Rows) != 1 || len(r.Steps) != 0 || r.Title != "订阅 5 个 Mod" || r.Obj != "" || strings.Contains(r.Line, "\n") || !strings.HasPrefix(r.Line, "完成 1，失败 4：") {
		t.Fatalf("row %+v", r)
	}
	if strings.Contains(r.Say, "http") || strings.Contains(r.Say, "failed:") || strings.Count(r.Say, "作者删除或设为私有") != 1 || len(strings.Split(r.Say, "\n")) != 3 {
		t.Fatalf("summary %q", r.Say)
	}
	if r.Items[1].Why != "已从创意工坊下架（作者删除或设为私有）" || r.Items[0].Why != "" || r.Items[1].WorkshopURL != "https://steamcommunity.com/sharedfiles/filedetails/?id=2" {
		t.Fatalf("items %+v", r.Items)
	}
	for _, a := range r.Acts {
		if a.ID == "retry" && (a.Label != "重试 1 个" || !a.Primary) {
			t.Fatalf("retry %+v", a)
		}
	}
	task.Items = task.Items[1:3]
	task.State = app.TaskFailed
	r = BuildTasks([]app.Task{task}, time.Now()).Rows[0]
	for _, a := range r.Acts {
		if a.ID == "retry" {
			t.Fatal("permanent failure offers retry")
		}
		if a.ID == "goto" && (!a.Primary || a.Label != "前往更新") {
			t.Fatalf("goto %+v", a)
		}
	}
	task.Items = task.Items[:1]
	task.Items[0].Name = "Named Mod"
	r = BuildTasks([]app.Task{task}, time.Now()).Rows[0]
	if r.Title != "订阅 Named Mod" {
		t.Fatalf("single title %q", r.Title)
	}
	task.State = app.TaskRunning
	r = BuildTasks([]app.Task{task}, time.Now()).Rows[0]
	if len(r.Acts) == 0 || r.Acts[0].ID != "stop" || r.Acts[0].Label != "取消" {
		t.Fatalf("cancel %+v", r.Acts)
	}
	SetLang("en")
	r = BuildTasks([]app.Task{task}, time.Now()).Rows[0]
	if r.Title != "subscribe Named Mod" || r.Items[0].Why != "removed from the Workshop (the author deleted it or made it private)" {
		t.Fatalf("English %+v", r)
	}
}

func TestSubscriptionProgressUsesByteWeights(t *testing.T) {
	t.Parallel()
	task := app.Task{Kind: TaskSubscribe, Lane: app.LaneSelf, State: app.TaskRunning}
	task.SetWorkshopProgress([]app.TaskItem{{Ref: "ws:1", State: app.ItemOK, Size: 100}, {Ref: "ws:2", Name: "Big Mod", State: app.ItemRun, Size: 300}})
	r := BuildTasks([]app.Task{task}, time.Now()).Rows[0]
	if r.Pct != 25 || r.Count != "1 / 2" || !strings.Contains(r.Line, "Big Mod") {
		t.Fatalf("progress %+v", r)
	}
}

// A source check names the source it checked, not "RimArcho" (VM 2026-11-23: the status
// line read "check for updates  RimArcho: 259 have updates" after Updates › Steam Workshop).
func TestSourceCheckTaskNamesItsSource(t *testing.T) {
	p := BuildTasks([]app.Task{
		{ID: "1", Kind: TaskSourceCheck, Obj: "RimArcho", Args: []string{"source-check", app.LibWorkshop}, Lane: app.LaneSelf, State: app.TaskDone, Result: "259 have updates"},
		{ID: "2", Kind: TaskSourceCheck, Obj: "RimArcho", Args: []string{"source-check", app.LibSteamCMD}, Lane: app.LaneSelf, State: app.TaskDone, Result: "up to date"},
	}, time.Now())
	objs := map[string]string{}
	for _, r := range p.Rows {
		objs[r.ID] = r.Obj
		if strings.Contains(TaskStatus(r), "RimArcho") {
			t.Fatalf("status names RimArcho: %q", TaskStatus(r))
		}
	}
	if objs["1"] != "Steam Workshop (client)" || objs["2"] != "SteamCMD" {
		t.Fatalf("objects %v", objs)
	}
}

// 11 §3: an item anonymous download was denied, with no account saved, says it needs one.
func TestSubscriptionNeedsAccountReason(t *testing.T) {
	SetLang("zh")
	defer SetLang("en")
	it := app.TaskItem{Ref: "ws:5", Name: "Denied Mod", State: app.ItemFail, FailureKind: "account", Permanent: true}
	if got := subscriptionReason(it, false); got != "需要 Steam 账号" {
		t.Fatalf("short %q", got)
	}
	if got := subscriptionReason(it, true); got != "匿名下载被拒绝，这个条目需要 Steam 账号（设置 › 账号）" {
		t.Fatalf("full %q", got)
	}
	if got := Text("note: " + fmt.Sprintf(app.WorkshopAccountNote, 2, "player", "8200, 8300")); got != "注意：2 个条目匿名下载被拒绝，需要 Steam 账号，SteamCMD 为它们用 player 登录：8200, 8300" {
		t.Fatalf("note %q", got)
	}
}
