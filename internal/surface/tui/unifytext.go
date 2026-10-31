package tui

import "github.com/ToumanLin/RimArcho/internal/view"

// The TUI's text for the filter bar, the key table, the row menu and Mod 库存 (06 §2.5,
// §2.9, §3.5, §8.5).
func init() {
	view.Register(map[string]string{
		"the arrows only move; keys that change things act at once, u undoes": "方向键只移动光标、焦点和滚动位置；改动东西的键立即生效，u 撤销",
		"enter: %s, esc: cancel":                       "enter：%s，esc：取消",
		"this cannot be done now, esc goes back":       "现在做不了，esc 返回",
		"nothing to unsubscribe or recycle here":       "这里没有可退订或移到回收站的",
		"one list only, P makes another":               "只有一个列表，P 新建",
		"%s filter":                                    "%s 筛选",
		"menu":                                         "菜单",
		"none, g adds":                                 "无，按 g 添加",
		"this copy cannot be used in the current list": "这一份不能用在当前列表里",
		"n writes a note on this copy":                 "n 给这一份写备注",
		"sort by":                                      "排序",
		"again reverses":                               "再选一次反过来",
		"open / close":                                 "展开 / 收起",
		"complete":                                     "补全",
		"drops the last filter":                        "去掉最后一个条件",
		"group":                                        "分组",
		"value":                                        "值",
		"only these":                                   "只看这些",
		"leave out":                                    "去掉",
		"type":                                         "输入",
		"find a value":                                 "找一个值",
		"search: words, tag:, state:, source:, used:, -not": "搜索：词、tag:、state:、source:、used:、-排除",
	})
}
