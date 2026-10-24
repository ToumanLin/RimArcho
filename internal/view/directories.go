package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
)

func DirectoryLabel(d app.ManagedDirectory) string {
	switch d.Kind {
	case "game":
		return Tf("game folder (%s)", d.Label)
	case "workshop":
		return T("Workshop folder")
	case "steamcmd":
		return Tf("SteamCMD folder (%s)", d.Label)
	case "extra":
		return Tf("other mod folder (%s)", d.Label)
	case "lock":
		return T("lock library")
	case "packs":
		return T("where packs are kept")
	case "monitor":
		return T("Archo Monitor folder")
	case "tool", "steamcmd-tool":
		return T("SteamCMD program folder")
	default:
		return d.Label
	}
}

func DirectoryNeedsLocation(d app.ManagedDirectory) bool {
	return d.Status != "ok" && d.Status != "unused"
}

// DirectoryGroup lists actual resolved paths, including the defaults before first use.
func DirectoryGroup(a *app.App) SettingGroup {
	g := SettingGroup{Title: T("managed directories")}
	dirs, err := a.ManagedDirectories()
	if err != nil {
		g.Items = append(g.Items, SettingItem{Label: T("directory check failed"), Note: err.Error()})
		return g
	}
	// One note for the whole group; a row only says what is different about it.
	g.Note = T("locate it after moving it yourself, or move it to a new location")
	for _, d := range dirs {
		note := ""
		if DirectoryNeedsLocation(d) {
			note = T("directory unavailable; enter its new location, or reconnect the drive")
		} else if d.Status == "unused" {
			note = T("not used yet")
		}
		item := SettingItem{Label: DirectoryLabel(d), Value: d.Path, Edit: "directory", Directory: d.ID,
			Argv: []string{"folder", "set", d.ID, "{}"}, MoveArgv: []string{"folder", "move", d.ID, "{}"}, Note: note}
		if d.Kind == "steamcmd" || d.Kind == "extra" {
			item.Actions = []SettingAction{{Label: T("remove"), Argv: []string{"folder", "remove", "src:" + d.Label},
				Confirm: T("RimArcho forgets this folder; the files stay where they are")}}
		}
		g.Items = append(g.Items, item)
	}
	return g
}

func directoryNeeds(a *app.App) []Need {
	dirs, err := a.ManagedDirectories()
	if err != nil {
		return []Need{{Group: GroupConfirm, Title: T("directory check failed"), Text: err.Error(), Why: err.Error(), Status: StatusWarn, Counted: true}}
	}
	var out []Need
	for _, d := range dirs {
		if !DirectoryNeedsLocation(d) {
			continue
		}
		title := Tf("locate %s", DirectoryLabel(d))
		why := T("the saved directory is unavailable or has changed; enter its new location in settings. You can keep playing")
		out = append(out, Need{Group: GroupConfirm, Title: title, Text: title + "  " + d.Path, Why: why + "  " + d.Path,
			Status: StatusWarn, Counted: true, Actions: []Action{{Kind: ActLocateDirectory, Key: "o", Label: T("locate directory"), Refs: []string{d.ID}, Lines: []string{d.Path}},
				{Kind: ActView, Key: "v", Label: T("directory details"), Argv: []string{"folder", "check"}}}})
	}
	return out
}

func init() {
	Register(map[string]string{
		"choose parent folder…":   "选择目标的上级目录…",
		"SteamCMD program folder": "SteamCMD 程序目录",
		"managed directories":     "RimArcho 管理的目录",
		"game folder (%s)":        "游戏目录（%s）",
		"SteamCMD folder (%s)":    "SteamCMD 目录（%s）",
		"other mod folder (%s)":   "其它 Mod 目录（%s）",
		"locate it after moving it yourself, or move it to a new location":      "自行移动后重新定位，或让 RimArcho 移动到新位置",
		"directory unavailable; enter its new location, or reconnect the drive": "目录无法访问；请填写新位置，或重新连接磁盘",
		"not used yet":           "尚未使用",
		"directory check failed": "目录检查失败",
		"locate %s":              "重新定位：%s",
		"locate directory":       "重新定位",
		"directory details":      "目录详情",
		"the saved directory is unavailable or has changed; enter its new location in settings. You can keep playing": "记录的目录无法访问或已有变化；请填写新位置。你可以继续游玩",
		"some directories need their new location":                                                               "有目录需要重新定位",
		"directories are checked each time RimArcho starts; you can skip this and locate them in settings later": "每次启动都会检查目录；可以暂时跳过，稍后到设置中重新定位",
		"locate":             "重新定位",
		"move to…":           "移动到…",
		"new directory path": "新目录路径",
		"move this directory and its files to the new path?": "将这个目录及其中的文件移动到新路径？",
		"skip for now": "稍后处理",
	})
}
