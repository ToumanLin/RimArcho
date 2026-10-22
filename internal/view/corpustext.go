package view

// Text of the log corpus's findings in 能不能跑, the launch hint, the sort's status line and
// the pattern card (03 §4.3, §6.8; 06 §2.3, §2.4, §2.10).
func init() {
	Register(map[string]string{
		// the 社区日志 chip
		"shared logs":         "社区日志",
		"%d of %d (%d%%)":     "%d/%d（%d%%）",
		"seen in shared logs": "分享的日志里见过",
		"errors with both in %s of shared logs, alone in %s":           "一起启用时 %s 出错，单独时 %s",
		"version %s errors in %s of shared logs, other versions in %s": "%s 版 %s 出错，其他版本 %s",
		"errors without it in %s of shared logs":                       "缺了它时 %s 出错",
		"errors in %s of the shared logs that enable it":               "启用它的日志里 %s 出错",
		"load-order constraint strength":                               "排序要求的强弱",
		// the corpus's items
		"probably missing: %s": "可能缺少前置：%s",
		"%s does not declare it, but in shared logs it fails without it. It is not installed.": "%s 没有声明它，但分享的日志里缺了它就会出错。它没有安装。",
		"%s does not declare it, but in shared logs it fails without it. It is not enabled.":   "%s 没有声明它，但分享的日志里缺了它就会出错。它没有启用。",
		"%s and %s often fail together": "%s 和 %s 一起时常出错",
		"in shared logs this pair produces errors far more often than either one alone. Disable one if you see errors.": "分享的日志里，这两个一起启用时出错远比单独时多。看到报错就停用其中一个。",
		"this version of %s often fails": "这个版本的 %s 常出错",
		"in shared logs this version produces errors; other versions did not. Updating it usually helps.": "分享的日志里这个版本会出错，其他版本不会。更新它通常就好了。",
		"this version of %s often produces errors (seen in shared logs); other versions did not":          "这个版本的 %s 经常报错（玩家分享的日志中常见），其他版本没有",
		"%s has a known error: this is the mod's own problem":                                             "%s 有一个已知错误：这是 Mod 自己的问题",
		"see the error": "看这个错误",
		"not your list's: shared logs show it whatever else is enabled; an update may fix it": "不是你的列表的问题：不管还启用了什么，分享的日志里都会出现；更新它可能就好了",
		// order strength (04 §5.3)
		"a strong requirement (a dependency or loadBefore): breaking it goes with more errors in shared logs; sorting puts it right": "强要求（依赖或 loadBefore）：分享的日志里违反它时出错更多；自动排序能排好",
		"%d weaker order preferences are not met": "%d 条较弱的顺序要求没满足",
		"loadAfter and community rules: breaking them showed no more errors in shared logs; sorting puts them right": "loadAfter 和社区规则：分享的日志里违反它们并没有多出错；自动排序会顺带排好",
		"sorted: %d moved · strong requirements put right: %d":                                                       "已排序：移动了 %d 个 · 排好的强要求：%d 条",
		"sorted: %d moved · only weaker preferences were out of order":                                               "已排序：移动了 %d 个 · 只有较弱的顺序要求没满足",
		// the launch hint (06 §2.3)
		"%s, and %d more like it": "%s，另有 %d 项同类",
		"watch for: %s":           "留意：%s",
		"patches that find nothing usually fail with red errors: such mods were error sources in 37% of logs (4.5% by chance)":                    "落空的补丁通常会报红字：分享的日志里这样的 Mod 有 37% 出错（随便挑一个是 4.5%）",
		"a definition built on a missing parent does not load: such mods were error sources in 16% of logs (4.1% by chance)":                      "父定义不存在时这个定义加载不了：分享的日志里这样的 Mod 有 16% 出错（随便挑一个是 4.1%）",
		"their authors say these mods do not work together: they were error sources in 26% of logs (7.3% by chance)":                              "作者说它们不能一起用：分享的日志里这样的 Mod 有 26% 出错（随便挑一个是 7.3%）",
		"a mod without its dependency fails in parts: such mods were error sources in 18% of logs (7.5% by chance)":                               "缺了前置的 Mod 部分功能会失效：分享的日志里这样的 Mod 有 18% 出错（随便挑一个是 7.5%）",
		"a mod without its dependency fails in parts: such mods were error sources in 10% of logs (4.4% by chance)":                               "缺了前置的 Mod 部分功能会失效：分享的日志里这样的 Mod 有 10% 出错（随便挑一个是 4.4%）",
		"the mod uses content from a mod that is not enabled, without declaring it: in shared logs it then errored in at least half of the cases": "这个 Mod 用到了没启用的 Mod 的内容，却没有声明：分享的日志里至少一半会出错",
		"in shared logs this mod errored in at least half of the lists with the other mod (or this version), far less without it":                 "分享的日志里，和另一个 Mod 一起（或用这个版本）时至少一半的列表出错，没有它时少得多",
	})
}
