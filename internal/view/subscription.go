package view

import (
	"slices"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

func appendSubscriptionFields(fs []Field, r *subscribe.Record) []Field {
	for _, f := range SubscriptionFields(r) {
		if f.Kind == FieldWorkshop && slices.ContainsFunc(fs, func(existing Field) bool { return existing.Kind == FieldWorkshop }) {
			continue
		}
		fs = append(fs, f)
	}
	return fs
}

func SubscriptionResult(op *app.SubscribeOp) string {
	if op == nil {
		return ""
	}
	var lines []string
	for _, it := range op.Items {
		result := Text(it.Result)
		switch result {
		case "up to date":
			result = T("up to date")
		case "update available":
			result = T("update available")
		case "updated":
			result = T("updated")
		case "installed":
			result = T("subscription finished")
		case "moved to recycle bin":
			result = T("moved to the recycle bin")
		}
		lines = append(lines, strings.TrimSpace(it.Name+"  "+result))
	}
	for _, note := range op.Notes {
		if note == "Steam downloads it; it appears after the next scan." {
			note = T("Steam downloads it; it appears after the next scan")
		}
		lines = append(lines, Text(note))
	}
	return strings.Join(lines, "  ")
}

// SubscriptionFields are shared by both detail views; reading them never checks the network.
func SubscriptionFields(r *subscribe.Record) []Field {
	if r == nil {
		return nil
	}
	var fs []Field
	url := func(kind, label, value string) {
		if value == "" {
			return
		}
		f := Field{Kind: kind, Label: T(label), Value: value}
		if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") {
			f.Act, f.ActAt, f.URL = ActURL, T("web page"), value
		}
		fs = append(fs, f)
	}
	url("subscription-url", "subscription origin", r.URL)
	url("subscription-repo", "repository", r.Repo)
	if r.Kind == "steamcmd" {
		fs = append(fs, workshopField(r.WorkshopID))
		if !r.InstalledAt.IsZero() {
			fs = append(fs, Field{Kind: "subscription-installed", Label: T("last download"), Value: LongDate(r.InstalledAt)})
		}
		return fs
	}
	if r.Kind == "git" {
		tracking := T("default branch")
		if r.Ref != "" {
			tracking = T(r.RefType) + " " + r.Ref
		}
		fs = append(fs, Field{Kind: "subscription-tracking", Label: T("tracking"), Value: tracking})
	}
	installed := r.Installed
	if r.Kind == "git" && len(installed) > 7 {
		installed = installed[:7]
	}
	if !r.InstalledAt.IsZero() {
		installed += "  " + LongDate(r.InstalledAt)
	}
	fs = append(fs, Field{Kind: "subscription-installed", Label: T("installed version"), Value: installed})
	check := T("not checked")
	if !r.CheckedAt.IsZero() {
		check = checkedAt(r.CheckedAt) + "  " + T("up to date")
		if r.HasUpdate() {
			check = checkedAt(r.CheckedAt) + "  " + T("update available")
		}
	}
	if r.CheckError != "" {
		check = Tf("failed: %s", Text(r.CheckError))
		if !r.CheckedAt.IsZero() {
			check = checkedAt(r.CheckedAt) + "  " + check
		}
	}
	fs = append(fs, Field{Kind: "subscription-check", Label: T("last check"), Value: check})
	return fs
}

// checkedAt is a check time as the rest of the page writes dates: "10 月 24 日 12:49".
func checkedAt(t time.Time) string { return ShortDate(t) + " " + t.Local().Format("15:04") }

func init() {
	Register(map[string]string{
		"an account operation is running; try again when it finishes":           "账户操作正在进行，完成后请重试",
		"checking the selected mods…":                                           "正在检查所选 Mod…",
		"quick delete (cannot be undone)":                                       "快速删除（无法恢复）",
		"Quick delete skips the recycle bin. Deleted files cannot be restored.": "快速删除会跳过回收站，删除的文件无法恢复。",
		"permanently delete these mods?":                                        "永久删除这些 Mod？",
		"permanently delete":                                                    "永久删除",
		"up to date":                                                            "已是最新", "update available": "有更新", "not checked": "未检查",
		"subscription type": "类型", "Git repository": "Git 仓库", "Workshop item": "工坊条目", "URL or Workshop ID": "地址或工坊 ID",
		"tracking name (blank: default branch)": "跟踪名称（留空为默认分支）", "use SteamCMD (only without the Steam client)": "用 SteamCMD（只在没有 Steam 客户端时用）",
		"downloaded zip or folder": "下载好的 zip 或文件夹", "choose zip…": "选择 zip…", "choose folder…": "选择文件夹…", "read page": "读取页面",
		"subscribe with Git: %s": "用 Git 订阅 %s", "choose downloaded files first": "先选择下载好的文件", "choose downloaded zip": "选择下载好的 zip",
		"choose a repository or downloaded files first": "先选择仓库或下载好的文件", "opened the page": "已打开页面",
		"subscription origin": "来处", "repository": "仓库", "tracking": "跟踪", "branch": "分支", "tag": "标签", "commit": "提交",
		"default branch": "默认分支", "installed version": "已装", "last check": "上次检查", "last download": "上次下载",
		"download page": "下载页", "source update available": "来源有更新", "failed: %s": "失败：%s",
		"remove subscription": "撤掉订阅", "check for updates": "检查更新", "update subscription": "更新", "open download page": "打开下载页",
		"subscribe…": "订阅…", "subscribe": "订阅", "subscription finished": "订阅已完成",
		"Steam downloads it; it appears after the next scan":         "Steam 正在下载，下次扫描后会出现",
		"page items skipped; choose downloaded files to update them": "已跳过下载页条目，选择下载好的文件来更新",
	})
}
