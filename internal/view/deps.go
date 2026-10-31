package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
)

// DependencyGroup shows the selected copies and commands for managing downloaded tools.
func DependencyGroup(a *app.App) SettingGroup {
	g := SettingGroup{Title: T("external tools"), Note: lastChecked(a, a.AutomaticStatus("tools").Checked)}
	labels := map[string]string{deps.Todds: "todds", deps.SteamCMD: "SteamCMD"}
	for _, i := range a.Deps().All() {
		item := SettingItem{Label: labels[i.Name], Value: T("not installed"), Note: i.Path, NotePaths: []string{i.Path}}
		if i.Version != "" {
			item.Value = i.Version
		} else if i.OK {
			item.Value = T("installed")
		}
		if i.Err != "" && i.Path != "" {
			item.Note += "  " + Humanize(nil, T(i.Err))
		}
		item.Note = strings.TrimSpace(item.Note)
		update := a.ToolUpdate(i.Name)
		if update.Pending && i.Own {
			item.ValueNote = Tf("new version available: %s", update.Latest)
			if !update.Compatible {
				item.Note += "  " + Tf("RimArcho supports todds %s; the new version has not been verified.", i.Version)
			}
		}
		if i.ConfigKey != "" {
			item.Edit, item.Argv = "path", []string{"config", "set", i.ConfigKey, "{}"}
			value := a.Store.Meta("cfg:" + i.ConfigKey)
			item.EditValue, item.File = &value, true
		}
		if i.Installable {
			label := T("install tool")
			if i.Path != "" {
				label = T("update tool")
			}
			item.Actions = append(item.Actions, SettingAction{Label: label, Argv: []string{"deps", "install", i.Name},
				Confirm: Tf("download %s into RimArcho's own folder?", labels[i.Name])})
			if update.Pending && !update.Compatible && i.Own {
				item.Actions = nil
			}
			if i.Own && a.CheckDepRemoval(i.Name) == nil {
				item.Actions = append(item.Actions, SettingAction{Label: T("remove"), Argv: []string{"deps", "remove", i.Name},
					Confirm: Tf("remove RimArcho's own copy of %s?", labels[i.Name])})
			}
		}
		if item.EditValue != nil && *item.EditValue != "" {
			item.Actions = append(item.Actions, SettingAction{Label: T("use default"), Argv: []string{"config", "set", i.ConfigKey, ""}})
		}
		g.Items = append(g.Items, item)
	}
	return g
}

func init() {
	Register(map[string]string{
		"external tools": "外部工具",
		"use default":    "使用默认",
		"installed":      "已安装",
		"install tool":   "安装工具",
		"update tool":    "更新工具",
		"download %s into RimArcho's own folder?": "将 %s 下载到 RimArcho 的专用目录？",
		"remove RimArcho's own copy of %s?":       "移除 RimArcho 下载的 %s 副本？",
		"remove external tool":                    "移除外部工具",
		"choose a program":                        "选择程序",
	})
}
