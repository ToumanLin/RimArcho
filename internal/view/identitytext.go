package view

func init() {
	Register(map[string]string{
		"do not list as a copy":                                                  "不列为副本",
		"%s is now listed independently":                                         "已将 %s 单独列出",
		"choose one copy to list independently":                                  "请选择一份要单独列出的副本",
		"this copy is already listed independently or is no longer available":    "这一份已单独列出，或已不在 Mod 库中",
		"list this copy as an independent mod; package ID conflicts still apply": "将这一份作为独立 Mod 列出；同包名的加载冲突仍然存在",
	})
}
