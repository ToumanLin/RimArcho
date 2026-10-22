package view

// The text of environments and the shared mod folders (02 §0): the environment page, the
// settings page's Mod 文件夹, and the add-environment guide's first step.
func init() {
	Register(map[string]string{
		"unknown":                            "未知",
		"game":                               "游戏",
		"game version":                       "游戏版本",
		"saves":                              "存档",
		"shared (the game's default folder)": "共用默认",
		"its own":                            "独立",
		"a folder you chose":                 "自选目录",
		"the game's own exe":                 "直接启动游戏",
		"mod folders":                        "Mod 文件夹",
		"mod folder":                         "Mod 文件夹",
		"SteamCMD folder":                    "SteamCMD 文件夹",
		"Archo Monitor folder":               "Archo Monitor 文件夹",
		"lock library":                       "锁定库",
		"lock library (%s, older)":           "锁定库（%s，旧）",
		"not there now":                      "现在不在",
		"moving to %s waits for your answer": "移到 %s，等你确认",
		"remove":                             "移除",
		"use it":                             "使用",
		"Steam has one at %s":                "Steam 里有一个：%s",
		"none found in the Steam libraries":  "Steam 库里没有找到",
		"choose…":                            "选择…",
		"choose a folder":                    "选择文件夹",
		"no folder dialog here; type the path": "这里不能打开文件夹对话框，请输入路径",
		"an environment is one game folder: its version, how it starts, its saves": "一个环境就是一个游戏目录：它的版本、启动方式和存档",
		// the guide's first step
		"a game you have":                   "已有的游戏",
		"download a version":                "下载一个版本",
		"the RimWorld installs found on this computer; tick the ones to add": "这台电脑上找到的 RimWorld；勾选要加入的",
		"or a game folder":                  "或者一个游戏目录",
		"reading the game's own mods…":      "正在读取游戏自己的 Mod…",
		"%s: %d mods in its Data and Mods":  "%s：Data 和 Mods 里有 %d 个 Mod",
	})
}
