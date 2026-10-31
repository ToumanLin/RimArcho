package view

func init() {
	Register(map[string]string{
		"window controls":                   "窗口控制",
		"minimize window":                   "最小化窗口",
		"maximize window":                   "最大化窗口",
		"restore window":                    "还原窗口",
		"close window":                      "关闭窗口",
		"complete":                          "补全",
		"running…":                          "正在运行…",
		"archo command":                     "archo 命令",
		"run log --follow in a terminal":    "请在终端里运行 log --follow",
		"the last command is still running": "上一条命令还在运行",
	})
}
