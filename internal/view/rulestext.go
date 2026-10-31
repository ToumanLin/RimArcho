package view

// The text of the editor's 排序规则 (06 §2.5, 04 §6.1).
func init() {
	Register(map[string]string{
		"load-order rules":                                "排序规则",
		"%s loads after %s":                               "%s 排在 %s 之后",
		"%s loads before %s":                              "%s 排在 %s 之前",
		"%s loads right after %s":                         "%s 紧跟在 %s 之后",
		"%s is incompatible with %s":                      "%s 与 %s 不兼容",
		"%s loads as early as it can":                     "%s 尽量靠前",
		"%s loads as late as it can":                      "%s 尽量靠后",
		"ignore %%s's About.xml: %s %s":                   "忽略 %%s 在 About.xml 里的 %s %s",
		"ignore that %s loads after the official content": "忽略 %s 要排在官方内容之后",
		"ignore the community rule: %s loads first":       "忽略社区规则：%s 置顶",
		"ignore the community rule: %s loads last":        "忽略社区规则：%s 置底",
		"ignore the community rule: %s %s %s":             "忽略社区规则：%s %s %s",
		"a mod that is no longer installed":               "已不在的 Mod",
		"choose the other mod":                            "先选另一个 Mod",
		"rule added: %s":                                  "已加规则：%s",
		"rule changed: %s":                                "已改规则：%s",
		"rule removed: %s":                                "已删除规则：%s",
		"nothing to undo":                                 "没有可撤销的",
		"rule restored: %s":                               "已恢复规则：%s",
	})
}
