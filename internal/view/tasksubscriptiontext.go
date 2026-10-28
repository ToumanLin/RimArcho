package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func subscriptionReason(it app.TaskItem, full bool) string {
	kind := it.FailureKind
	if kind == "" && strings.Contains(it.Why, "no longer on the Workshop") {
		kind = "missing"
	}
	if kind == "missing" {
		if full {
			return T("no longer on the Workshop (author deleted it or made it private)")
		}
		return T("no longer on the Workshop")
	}
	if kind == "" && (it.Why == "the Steam client is offline" || strings.Contains(it.Why, "(code 3)")) {
		kind = "offline"
	}
	if kind == "offline" {
		if full {
			return T("the Steam client is offline: restart Steam, then retry (SteamCMD signing in with the same account signs Steam out)")
		}
		return T("the Steam client is offline")
	}
	key := map[string]string{
		"permission": "access denied", "ownership": "account does not own this content",
		"disk": "not enough disk space or unable to write", "network": "connection failed or timed out",
		"crash": "SteamCMD exited before confirming the download", "password": "check the Steam password",
		"guard": "Steam Guard verification required", "login": "Steam login failed", "anonymous-login": "Steam login failed",
	}[kind]
	if key != "" {
		return T(key)
	}
	if strings.HasPrefix(it.Ref, "ws:") {
		// Steam's own reason (workshopresult.go) says more than "not confirmed"
		if why := strings.TrimPrefix(it.Why, "failed: "); strings.HasPrefix(why, "Steam ") {
			return Text(why)
		}
		return T("download was not confirmed")
	}
	return Humanize(nil, strings.TrimPrefix(it.Why, "failed: "))
}

func subscriptionTaskText(t *app.Task, r *TaskRow, c map[string]int) {
	names := make([]string, len(t.Items))
	for i, it := range t.Items {
		names[i] = it.Name
	}
	r.Title, r.Obj = Tf("subscribe %s", TaskObj(names)), ""
	if t.State == app.TaskRunning {
		r.Line = Tf("downloading %s", t.Phase)
		if t.Phase == "" {
			r.Line = T("finishing the subscription")
		}
		r.Say = joinFacts(r.Count, r.Line)
		return
	}
	if !t.Ended() {
		return
	}
	r.Line = Tf("completed %d, failed %d", c[app.ItemOK], c[app.ItemFail])
	var order, short []string
	groups := map[string]int{}
	for _, it := range t.Items {
		if it.State != app.ItemFail {
			continue
		}
		long := subscriptionReason(it, true)
		if groups[long] == 0 {
			order = append(order, long)
			short = append(short, subscriptionReason(it, false))
		}
		groups[long]++
	}
	var lines []string
	for _, reason := range order {
		if reason == T("no longer on the Workshop (author deleted it or made it private)") {
			lines = append(lines, Tf("%d mods are no longer on the Workshop (deleted or private)", groups[reason]))
		} else {
			lines = append(lines, Tf("%d mods: %s", groups[reason], reason))
		}
	}
	if len(short) > 0 {
		reasons := short[0]
		for _, reason := range short[1:] {
			reasons = Tf("%s; %s", reasons, reason)
		}
		r.Line = Tf("%s: %s", r.Line, reasons)
	}
	if c[app.ItemSkip] > 0 {
		r.StateText = T("cancelled")
		r.Line = Tf("completed %d, cancelled %d", c[app.ItemOK], c[app.ItemSkip])
		lines = append([]string{r.Line}, lines...)
	}
	r.Say = strings.Join(lines, "\n")
	if r.Say == "" {
		r.Say = r.Line
		if t.Result == T("Steam downloads it; it appears after the next scan") {
			r.Say += "\n" + t.Result
		}
	}
}

func init() {
	Register(map[string]string{
		"subscribe %s":               "订阅 %s",
		"completed %d, failed %d":    "完成 %d，失败 %d",
		"completed %d, cancelled %d": "完成 %d，已取消 %d",
		"no longer on the Workshop":  "已不在创意工坊",
		"no longer on the Workshop (author deleted it or made it private)": "已不在创意工坊（作者删除或设为私有）",
		"%d mods are no longer on the Workshop (deleted or private)":       "%d 个 Mod 已不在创意工坊（作者删除或设为私有）",
		"%d mods: %s":                                       "%d 个 Mod：%s",
		"account does not own this content":                 "账户未拥有此内容",
		"not enough disk space or unable to write":          "磁盘不足或无法写入",
		"connection failed or timed out":                    "连接失败或超时",
		"SteamCMD exited before confirming the download":    "SteamCMD 异常退出，下载未确认",
		"check the Steam password":                          "请检查 Steam 密码",
		"Steam Guard verification required":                 "需要 Steam Guard 验证",
		"Steam login failed":                                "Steam 登录失败",
		"downloading %s":                                    "正在下载 %s",
		"finishing the subscription":                        "正在完成订阅",
		"access was denied; check account access":           "访问被拒绝，请检查账户权限",
		"cancelling the download; completed items are kept": "正在取消下载，已完成的条目会保留",
		"retry %d items":                                    "重试 %d 个",
		"continue %d items":                                 "继续 %d 个",
		"go to subscriptions and updates":                   "前往订阅与更新",
		"nothing to retry":                                  "没有可重试的条目",
		"cancelled":                                         "已取消",
		"%s; %s":                                            "%s；%s",
	})
}
