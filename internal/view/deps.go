package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
)

// DependencyGroup shows the selected copies and commands for managing downloaded tools.
func DependencyGroup(a *app.App) SettingGroup {
	g := SettingGroup{Title: T("external tools")}
	labels := map[string]string{deps.Font: T("bundled font"), deps.Todds: "todds", deps.SteamCMD: "SteamCMD", deps.WezTerm: "WezTerm"}
	for _, i := range a.Deps().All() {
		item := SettingItem{Label: labels[i.Name], Value: T("not installed"), Note: i.Path}
		if i.Version != "" {
			item.Value = i.Version
			if i.Name == deps.Font {
				count, _, _ := strings.Cut(i.Version, " ")
				item.Value = Tf("%s font files", count)
			}
		} else if i.OK {
			item.Value = T("installed")
		}
		if i.Err != "" && i.Path != "" {
			item.Note += "  " + Humanize(nil, T(i.Err))
		}
		if i.Name == deps.WezTerm {
			item.Note += "  " + T("only needed for the terminal interface (archo tui)")
		}
		item.Note = strings.TrimSpace(item.Note)
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
			if i.Own && a.CheckDepRemoval(i.Name) == nil {
				item.Actions = append(item.Actions, SettingAction{Label: T("remove"), Argv: []string{"deps", "remove", i.Name},
					Confirm: Tf("remove RimArcho's own copy of %s?", labels[i.Name])})
			}
		}
		g.Items = append(g.Items, item)
	}
	return g
}

func init() {
	Register(map[string]string{
		"external tools":              "外部工具",
		"bundled font":                "随附字体",
		"%s font files":               "%s 个字体文件",
		"the bundled font is missing": "随附字体缺失",
		"installed":                   "已安装",
		"install tool":                "安装工具",
		"update tool":                 "更新工具",
		"only needed for the terminal interface (archo tui)": "仅终端界面（archo tui）需要",
		"download %s into RimArcho's own folder?":            "将 %s 下载到 RimArcho 的专用目录？",
		"remove RimArcho's own copy of %s?":                  "移除 RimArcho 下载的 %s 副本？",
		"download WezTerm":                                   "下载 WezTerm",
		"remove external tool":                               "移除外部工具",
		"choose a program":                                   "选择程序",
	})
}
