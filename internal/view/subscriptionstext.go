package view

func init() {
	Register(map[string]string{
		"subscriptions and updates":           "订阅与更新",
		"subscriptions and updates, game log": "订阅与更新、游戏日志",
		"the next field or button":            "下一个字段或按钮",
		"sources":                             "来源",
		"pending updates":                     "可更新",
		"change history":                      "变化记录",
		"Steam Workshop (client)":             "Steam 工坊（客户端）",
		"download page / LoversLab":           "下载页 / LoversLab",
		"local (no source)":                   "本地（无来源）",
		"accounts":                            "账户",
		"check complete":                      "检查完成",
		"update all":                          "全部更新",
		"view changes":                        "查看变化",
		"update / view changes":               "更新 / 查看变化",
		"select updates":                      "选择可更新项",
		"sources, updates, history":           "来源、可更新、变化记录",
		"close the panel, clear selection, then show every mod": "先收起面板，再清除选择，再去掉只看条件",
		"no known updates; check a source above":                "没有已知更新，可在来源里检查更新",
		"Steam will download this Workshop update":              "Steam 客户端将下载此工坊更新，完成后按 R 重新扫描",
	})
}
