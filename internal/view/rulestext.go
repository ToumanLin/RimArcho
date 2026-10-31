package view

// The text of the editor's 排序规则 (06 §2.5, 04 §6.1).
func init() {
	Register(map[string]string{
		"show it in the list": "在列表中定位",
		"edit":                "编辑",
		// the TUI's 排序规则 screen (KeyHelp RulesPage)
		"only the current list / all load-order rules":  "只看当前列表 / 全部排序规则",
		"add a rule for the mod at the editor's cursor": "给编辑器光标所在的 Mod 加一条规则",
		"change the rule": "改这条规则",
		"remove the rule; u right after puts it back":     "删除这条规则；紧接着按 u 放回",
		"show its mod in the list":                        "在列表中定位它的 Mod",
		"in the form: the next / previous field":          "表单里：下一项 / 上一项",
		"in the form: the kind":                           "表单里：种类",
		"close the form; back to the editor":              "关闭表单；回到编辑器",
		"choose a mod in the lists first":                 "先在列表里选一个 Mod",
		"for the mod at the cursor":                       "给光标所在的 Mod",
		"the other mod":                                   "另一个 Mod",
		"note (optional)":                                 "备注（可不填）",
		"add":                                             "添加",
		"only the current list":                           "只看当前列表",
		"all load-order rules":                            "全部排序规则",
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
