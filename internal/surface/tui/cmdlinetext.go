package tui

import "github.com/ToumanLin/RimArcho/internal/view"

// The command line's text (06 §9.4).
func init() {
	view.Register(map[string]string{
		"command line":                      "命令行",
		"nothing to complete":               "没有可补全的",
		"the last command is still running": "上一条命令还在运行",
		"running archo %s…":                 "正在运行 archo %s…",
		"the %d selected mods":              "选中的 %d 个 Mod",
		"the command failed":                "命令失败",
		"another":                           "换一个",
		"take it":                           "选定",
		"put the word back":                 "恢复原来的词",
		"the status line already follows the game; run it in a terminal for a scrolling log": "TUI 的状态行已经在跟着游戏；要看滚动的日志就在终端里运行",
	})
}
