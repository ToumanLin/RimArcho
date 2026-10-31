package gui

import "github.com/ToumanLin/RimArcho/internal/view"

// The text of the editor's 排序规则 fold (rules.js, 06 §2.5).
func init() {
	view.Register(map[string]string{
		"choose a mod in the lists first": "先在列表里选一个 Mod",
		"for the mod at the cursor":       "给光标所在的 Mod",
		"the other mod":                   "另一个 Mod",
		"note (optional)":                 "备注（可不填）",
		"add":                             "添加",
	})
}
