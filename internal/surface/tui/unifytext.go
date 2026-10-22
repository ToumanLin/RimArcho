package tui

import "github.com/ToumanLin/RimArcho/internal/view"

// The TUI's text for the filter bar, the key table, the row menu and Mod 库存 (06 §2.5,
// §2.9, §3.5, §8.5).
func init() {
	view.Register(map[string]string{
		"%s filter":             "%s 筛选",
		"complete":              "补全",
		"drops the last filter": "去掉最后一个条件",
		"group":                 "分组",
		"value":                 "值",
		"only these":            "只看这些",
		"leave out":             "去掉",
		"type":                  "输入",
		"find a value":          "找一个值",
		"search: words, tag:, state:, source:, used:, -not": "搜索：词、tag:、state:、source:、used:、-排除",
	})
}
