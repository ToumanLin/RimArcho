package tui

import (
	"fmt"
	"os"
	"strings"
)

// The TUI speaks Chinese or English (06 §1, "跟随配置"); command output shown in panels is
// the fixed-English CLI text (05 §1). Strings are looked up by their English text.

var lang = "en"

// SetLang selects the TUI language: "zh", "en" or "auto" (ARCHO_LANG, then the OS UI
// language).
func SetLang(v string) {
	if e := os.Getenv("ARCHO_LANG"); e != "" {
		v = e
	}
	if v == "" || v == "auto" {
		v = systemLang()
	}
	if strings.HasPrefix(strings.ToLower(v), "zh") {
		lang = "zh"
	} else {
		lang = "en"
	}
}

// T translates a TUI string.
func T(s string) string {
	if lang == "zh" {
		if z, ok := zhText[s]; ok {
			return z
		}
	}
	return s
}

// Tf translates a format string, then formats.
func Tf(f string, a ...any) string { return fmt.Sprintf(T(f), a...) }

var zhText = map[string]string{
	// header / tabs / status
	"no environment":      "无环境",
	"env: ":               "环境：",
	"[1]list":             "[1]列表",
	"[2]problems":         "[2]问题",
	"[3]changes":          "[3]变更",
	"active %d/%d":        "启用 %d/%d",
	"unsaved %d":          "未保存 %d",
	"saved":               "已保存",
	"problems —":          "问题 —",
	"problems %d":         "问题 %d",
	"changes %d":          "变更 %d",
	"questions %d":        "待确认 %d",
	"marked %d":           "已标记 %d",
	"scanning…":           "扫描中…",
	"external change v%d": "外部改动 v%d",
	"list saved for %s: game resets it (w to save)": "列表按 %s 保存：游戏会重置它（w 保存）",
	"search: ":       "搜索：",
	"  (esc clears)": "  （esc 清除）",
	": command   / search   space toggle   S sort   w save   L launch   ? help   q quit": ": 命令   / 搜索   空格 启停   S 排序   w 保存   L 启动   ? 帮助   q 退出",
	"error: ": "错误：",
	"Set up an environment first: archo env detect, archo env add <name> --detected": "请先设置环境：archo env detect，archo env add <名称> --detected",
	// list
	"ref":    "引用",
	"name":   "名称",
	"source": "来源",
	"ver":    "版本",
	"flags":  "标记",
	"  (nothing matches; esc clears the search, f changes the filter)": "  （无匹配；esc 清除搜索，f 切换筛选）",
	"missing":            "缺失",
	"residue":            "残留",
	"not loaded by game": "游戏不加载",
	"duplicate":          "重复",
	"copies":             "多副本",
	"about-disabled":     "About 已禁用",
	"filter: ":           "筛选：",
	"all":                "全部",
	"enabled":            "已启用",
	"disabled":           "未启用",
	"unloadable":         "不可加载",
	// panels / prompts
	"(%d–%d of %d, ↑/↓)":             "（%d–%d / 共 %d，↑/↓）",
	"enter/esc: close":               "enter/esc：关闭",
	"    n: cancel":                  "    n：取消",
	"help":                           "帮助",
	"scan failed":                    "扫描失败",
	"sort — preview":                 "排序 — 预览",
	"save to game — preview":         "保存到游戏 — 预览",
	"launch":                         "启动",
	"cancelled":                      "已取消",
	"only enabled mods can be moved": "只能移动已启用的 Mod",
	"move %s to (position | before <ref> | after <ref>): ":                             "把 %s 移到（位置 | before <引用> | after <引用>）：",
	"expected a position number":                                                       "需要一个位置数字",
	"expected: <n>, before <ref> or after <ref>":                                       "应为：<n>、before <引用> 或 after <引用>",
	"rule for %s (after|before|attach|incompatible <ref>): ":                           "为 %s 加规则（after|before|attach|incompatible <引用>）：",
	"expected: <kind> <ref>":                                                           "应为：<类型> <引用>",
	"no environments (archo env add)":                                                  "没有环境（archo env add）",
	"switch environment (enter to choose)":                                             "切换环境（enter 选择）",
	"problems: %s in the list (kept in ModsConfig.xml)\n":                              "问题：列表中有 %s（保留在 ModsConfig.xml 中）\n",
	"warning: %s in the Loadout match no installed mod":                                "警告：Loadout 中有 %s 未匹配到已安装的 Mod",
	"ModsConfig.xml changed outside RimArcho (v%d) — :loadout checkout v%d to load it": "ModsConfig.xml 在 RimArcho 之外被修改（v%d）— 用 :loadout checkout v%d 载入",
	"missing entry":                       "缺失条目",
	"entry":                               "条目",
	"add ignore rule %s? (y/n)":           "添加屏蔽规则 %s？(y/n)",
	"no suggested edge for this problem":  "此问题没有可屏蔽的建议边",
	"no problems":                         "没有问题",
	"analysis is stale — A to re-analyze": "分析结果已过期 — 按 A 重新分析",
	"not analyzed yet — A to analyze (child process)": "尚未分析 — 按 A 分析（子进程）",
	"analyzing…":                            "分析中…",
	"analysis failed":                       "分析失败",
	"severity: ":                            "级别：",
	"inspect":                               "检查",
	"problems %d/%d/%d":                     "问题 %d/%d/%d",
	"analysis done":                         "分析完成",
	"[ignore expired]":                      "[忽略已失效]",
	"evidence: ":                            "证据：",
	"error":                                 "错误",
	"warning":                               "警告",
	"info":                                  "提示",
	"showing ignored":                       "显示已忽略",
	"ignored %d (I shows)":                  "已忽略 %d（I 显示）",
	"no suggested command for this problem": "此问题没有建议命令",
	"suggestion":                            "建议",
	"(not a command; open it yourself)":     "（不是命令，请自行打开）",
	"run this command?":                     "运行这条命令？",
	"summary":                               "概要",
	"defs":                                  "Def",
	"patches":                               "Patch",
	"problems":                              "问题",
	"rules":                                 "规则",
	"history":                               "历史",
	"new log (l)":                           "新日志（l）",
	"no Player.log found":                   "未找到 Player.log",
	"analyze which log? (enter)":            "分析哪个日志？（enter）",
	"log analyzed: %d problems":             "日志分析完成：%d 个问题",
	"external ModsConfig.xml version ignored (it stays in loadout log)": "已忽略外部 ModsConfig.xml 版本（仍保留在 loadout log 中）",
	"… %d more": "… 另有 %d 项",
	"(semantic files unchanged: only other files changed)": "（语义文件未变，只有其他文件变化）",
	"(snapshot no longer stored)":                          "（快照已不在库中）",
	"(no file-level difference recorded for this event)":   "（此事件没有文件级差异记录）",
	"no changes":            "无变化",
	"no scan yet (R scans)": "尚未扫描（R 扫描）",
	"changed outside RimArcho, kept as v%d — L load into the workspace · I ignore": "被外部修改，已存为 v%d — L 载入工作区 · I 忽略",
	"score": "相似度",
	"identity questions %d — s same · c copy · u unrelated":                "身份待确认 %d 项 — s 是同一个 · c 是副本 · u 无关",
	"%d bool prefixes from %d mods can skip methods other mods also patch": "%d 个返回 bool 的前缀（来自 %d 个 mod）可能跳过其他 mod 也在补丁的方法",
	helpText: helpTextZh,
}

const helpTextZh = `列表视图（06 §3）
  ↑/↓ j/k  移动光标           PgUp/PgDn g/G  翻页 / 首尾
  space    启用 / 停用        J / K          下移 / 上移
  m        移动到 …           v              标记（多选）
  S        排序（先看差异）   w              保存到游戏（先看差异）
  u        撤销               ctrl+r         重做
  x        同谱系切换副本     L              启动游戏
  enter    检查               r              为此 Mod 添加规则
  /        搜索               f              筛选：全部/已启用/未启用/不可加载
  R        重新扫描（子进程）    l              分析 Player.log（或上一次的）
问题视图：enter 展开 / 检查   a 运行建议命令（tab 选择）   i 忽略
          I 显示已忽略   s 级别   o 检查该 Mod   A 重新分析（子进程）
变更视图：enter 文件差异   s/c/u 回答身份问题   L 载入 / I 忽略外部 ModsConfig.xml
视图：1 列表 · 2 问题 · 3 变更      E 切换环境      鼠标：滚轮滚动，点击选中
:        运行任意 archo 命令                 ? 帮助   q 退出`

// trFlags translates the space-joined row flags; multi-word flags are matched first.
func trFlags(f string) string {
	if lang != "zh" || f == "" {
		return f
	}
	f = strings.ReplaceAll(f, "not loaded by game", T("not loaded by game"))
	words := strings.Fields(f)
	for i, w := range words {
		words[i] = T(w)
	}
	return strings.Join(words, " ")
}
