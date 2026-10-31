package gui

import "github.com/ToumanLin/RimArcho/internal/view"

func init() {
	view.Register(map[string]string{
		"results":            "结果",
		"searching…":         "正在搜索…",
		"text to find":       "要找的文字",
		"case sensitive":     "区分大小写",
		"whole word":         "整词",
		"regular expression": "正则",
		"Def name":           "Def 名",
		"kind":               "种类",
		"C# code":            "C# 代码",
		"scope":              "范围",
		"+ Mod":              "+ Mod",
		"C# code searches only named mods: add one with + Mod first": "C# 代码只在点名的 Mod 里搜：先用 + Mod 加上",
		"choose a file kind":    "先选一种文件",
		"%d matches":            "%d 处",
		"+%d matches not shown": "+%d 处未显示",
		"nothing found":         "没有找到",
		"next page":             "下一页",
		"page %d":               "第 %d 页",
	})
}
