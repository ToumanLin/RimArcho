package view

func init() {
	Register(map[string]string{
		"show fewer": "收起",
		"search: words, tags:, source:, content:": "搜索：词、tags:、source:、content:",
		"copies":                    "副本",
		"available keys":            "可用条件",
		"available values":          "可用值",
		"no suggestions":            "没有匹配的候选",
		"view this copy":            "查看此副本",
		"use this copy in the list": "在列表中使用此副本",
		"view full details":         "查看完整详情",
	})
}
