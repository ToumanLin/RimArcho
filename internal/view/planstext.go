package view

// The plans' dialogs, in both interfaces (the window's plans.js, the TUI's plans.go).
func init() {
	Register(map[string]string{
		"name":                               "名称",
		"plan":                               "列表",
		"plans: switch, new, rename, delete": "列表：切换、新建、改名、删除",
		"delete %s":                          "删除 %s",
		"from an import: a file, a share code, a link…": "从导入：文件、分享码、链接…",
		"start from": "从哪里开始",
		"create":     "新建",
		"the game's list stays as it is until you write or launch": "写入游戏或启动之前，游戏里的列表不变",
		"rename the plan":                "列表改名",
		"rename %s":                      "给 %s 改名",
		"rename":                         "改名",
		"delete the plan":                "删除列表",
		"delete":                         "删除",
		"rename…":                        "改名…",
		"copy as a new plan…":            "复制成新列表…",
		"new plan…":                      "新列表…",
		"delete…":                        "删除…",
		"editing":                        "正在编辑",
		"rename %s…":                     "给 %s 改名…",
		"delete %s…":                     "删除 %s…",
		"the plan: rename, copy, delete": "列表：改名、复制、删除",
		"can it run › needs your answer": "能不能跑 › 要你确认",
		"switching plans writes no game file; writing to the game or launching does": "切换列表不改游戏文件；写入游戏或启动时才写",
		"in the game":   "游戏里是它",
		"as a new plan": "成为新列表",
		"%d mods; the current plan stays as it is": "%d 个 Mod；当前列表保持不变",
		"create the plan":                          "建立列表",
	})
}
