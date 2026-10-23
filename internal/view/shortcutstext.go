package view

// The shortcuts for one mod (06 §3.5) and the 变化 page narrowed to one mod (§2.9), in
// both interfaces.
func init() {
	Register(map[string]string{
		"no Workshop ID: not a Workshop mod":         "没有工坊 ID：不是创意工坊 Mod",
		"open the folder (the plan and the library)": "打开文件夹（列表和 Mod 库）",
		"Workshop page (the plan and the library)":   "工坊页面（列表和 Mod 库）",
		"opened the folder":                          "已打开文件夹",
		"RimArcho has not seen this copy: no folder": "RimArcho 没见过这一份：没有文件夹",
		"folder / Workshop page":                     "文件夹 / 工坊页面",
		"see all on the changes page":                "在变化页查看全部",
		"this mod's changes":                         "这个 Mod 的变化",
		"only %s":                                    "只看 %s",
		"every mod":                                  "所有 Mod",
		"esc shows every mod":                        "esc 显示所有 Mod",
		"no change recorded for this mod":            "这个 Mod 还没有记录到变化",
	})
}
