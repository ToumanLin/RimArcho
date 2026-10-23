package view

// Archo Monitor's text (settings.go, plans.go, live.go; 08 §9, 04 §1.5).
func init() {
	Register(map[string]string{
		"add Archo Monitor to plans": "自动把 Archo Monitor 加进列表",
		"new plans, and each plan once; after that it is a normal mod you can disable or move": "只管新列表和还没处理过的列表，每个列表只加一次；加进来以后它是普通 Mod，可以停用、移动",
		"Archo Monitor installed":                             "已安装的 Archo Monitor",
		"Archo Monitor added after Harmony (undo removes it)": "已在 Harmony 后面加入 Archo Monitor（撤销即可去掉）",
		"not installed":                                       "未安装",
		"%s · this run, %d times":                             "%s · 本次运行，%d 次",
		"Archo Monitor is ready":                              "Archo Monitor 已就绪",
		// the log page's companion section (logpage.go, monitorlog.go; 06 §2.10)
		"patch error": "补丁错误", "def config error": "Def 配置错误", "cross-reference error": "交叉引用错误",
		"XML load error": "XML 加载错误", "assembly conflict": "程序集冲突", "tick exception": "Tick 异常",
		"draw exception": "绘制异常", "null reference": "空引用", "out of memory": "内存不足",
		"missing texture": "贴图缺失", "Archo Monitor's own error": "Monitor 自身出错",
		"game %s": "游戏 %s", "since %s": "%s 起",
		"%s, no exit line (it crashed or was killed)":                                                               "%s 起 · 没有退出记录（崩溃或被强制关闭）",
		"full: no new entries after 32 MiB":                                                                         "已满：32 MiB 之后不再记新条目",
		"this plan has no Archo Monitor: errors will not say which file or def was loading":                         "这个列表没有 Archo Monitor：报错不会说明当时在加载哪个文件、哪个 Def",
		"Archo Monitor is not installed: errors will not say which file or def was loading (archo monitor install)": "Archo Monitor 未安装：报错不会说明当时在加载哪个文件、哪个 Def（archo monitor install）",
		"add Archo Monitor":                 "加入 Archo Monitor",
		"added Archo Monitor after Harmony": "已在 Harmony 后面加入 Archo Monitor",
		"Player.log · %d":                   "Player.log · %d", "%s in Player.log": "Player.log 里的 %s",
		"only in %s: the game had stopped writing Player.log": "只在 %s 里：游戏当时已不再写 Player.log",
		"no matching error in Player.log":                     "Player.log 里没有对应的报错",
		"pick an entry on the left":                           "选左边的一条",
		"nothing reported in this run":                        "这次运行没有报错",
	})
}
