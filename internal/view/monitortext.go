package view

// Archo Monitor's text (settings.go, plans.go, live.go; 08 §9, 04 §1.5).
func init() {
	Register(map[string]string{
		"add Archo Monitor to plans": "自动把 Archo Monitor 加进方案",
		"new plans, and each plan once; after that it is a normal mod you can disable or move": "只管新方案和还没处理过的方案，每个方案只加一次；加进来以后它是普通 Mod，可以停用、移动",
		"Archo Monitor installed":                             "已安装的 Archo Monitor",
		"Archo Monitor added after Harmony (undo removes it)": "已在 Harmony 后面加入 Archo Monitor（撤销即可去掉）",
		"not installed":                                       "未安装",
		"%s · this run, %d times":                             "%s · 本次运行，%d 次",
		"Archo Monitor is ready":                              "Archo Monitor 已就绪",
	})
}
