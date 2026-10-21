package view

// The shell's text (shell.go): the sidebar, the home page, the first-run guide.
func init() {
	Register(map[string]string{
		"home":                       "首页",
		"can it run":                 "能不能跑",
		"plans":                      "方案",
		"current list":               "当前列表",
		"mod library":                "Mod 库",
		"changes":                    "变化",
		"playing":                    "在玩",
		"%d mods enabled":            "启用了 %d 个 Mod",
		"%d not written":             "%d 项未写入",
		"not checked yet":            "还没检查",
		"%d problems break the game": "%d 个问题会出错",
		"ready to play":              "能跑",
		"checked when built":         "编译时检查过",
		"type the game folder":       "输入游戏目录",
		"%s does not look like a RimWorld folder (no game executable or Data/Core)": "%s 不像 RimWorld 的目录（没有游戏程序或 Data/Core）",
		"pack %s is in the packs folder; play it from the home page":                "整合包 %s 已放进整合包目录，可以在首页启动",
		"pack %s: %d installed, %d another version, %d missing":                     "整合包 %s：已安装 %d，版本不同 %d，缺少 %d",
		"its list is now the current list; u undoes":                                "它的列表已成为当前列表，u 可撤销",
	})
}
