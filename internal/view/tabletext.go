package view

func init() {
	Register(map[string]string{
		"pack name":                "包名",
		"load order":               "加载顺序",
		"version folders left out": "省掉的版本目录",
		"PNG count":                "PNG 数",
		"current size":             "现在的大小",
		"estimated size":           "估计的大小",
		"progress / state":         "进度 / 状态",
		"%d stale":                 "%d 个过期",
		"%d to convert":            "%d 个待转换",
		"%d own DDS":               "%d 个自带 DDS",
		"up to date":               "已是最新",
		"reset width: %s":          "恢复 %s 的宽度",
		"the headers: sort and columns; < > = changes or resets the sorted column's width": "表头：排序和列；< > = 改排序列宽度或恢复",
		"close the preview / back to the list":                                             "收起清单 / 回到列表",
	})
}
