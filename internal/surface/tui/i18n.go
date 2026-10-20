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
	// top bar, columns, status and hint lines (06 §2.3: short, verb first)
	"no environment":     "无环境",
	"needs attention %d": "需要处理 %d",
	"needs attention":    "需要处理",
	"unsaved %d":         "未保存 %d",
	"launch":             "启动",
	"recent changes":     "最近变化",
	"details":            "详情",
	"Disabled":           "未启用",
	"Enabled":            "已启用",
	"Staging":            "暂存",
	"(not installed)":    "（未安装）",
	"nothing matches · esc clears the filter":         "没有匹配的 Mod · esc 清除筛选",
	"staging is empty · t puts the selected mod here": "暂存是空的 · 按 t 把选中的 Mod 放进来",
	"no mod is enabled · space enables one":           "还没有启用 Mod · 按 space 启用",
	"every mod is enabled":                            "所有 Mod 都已启用",
	"staging is empty":                                "暂存是空的",
	"scanning…":                                       "扫描中…",
	"analyzing…":                                      "分析中…",
	"error: ":                                         "错误：",
	"Set up an environment first: archo env detect, archo env add <name> --detected": "请先设置环境：archo env detect，archo env add <名称> --detected",
	// hint line
	"done": "完成", "clear": "清除", "move": "移动", "choose": "选择", "confirm": "确认", "cancel": "取消",
	"back": "返回", "scroll": "滚动", "section": "切换子页", "expand": "展开", "run the suggestion": "执行建议",
	"ignore": "忽略", "choose a log": "选择日志", "re-analyze": "重新分析", "file changes": "文件变化",
	"enable": "启用", "disable": "停用", "stage": "暂存", "unstage": "移出暂存", "put back": "放回",
	"put all back": "全部放回", "filter": "筛选", "columns": "切换列", "lists": "切换列表", "sort": "排序",
	"save": "保存", "undo": "撤销", "help": "帮助",
	// results (status line)
	"enabled %s":                                "启用了 %s",
	"disabled %s":                               "停用了 %s",
	"staged %s":                                 "暂存了 %s",
	"took %s out of staging":                    "把 %s 移出了暂存",
	"put %s back in place":                      "把 %s 放回了原位置",
	"%s is back at number %d":                   "%s 放回了第 %d 个",
	"%s is now number %d":                       "%s 现在是第 %d 个",
	"enabled %s at number %d":                   "启用了 %s，放在第 %d 个",
	"undid the last change":                     "撤销了上一步",
	"redid the change":                          "重做了一步",
	"sorted the list":                           "排好了顺序",
	"saved to the game":                         "保存到了游戏",
	"the game is starting":                      "正在启动游戏",
	"rule added":                                "添加了规则",
	"swapped %s for another copy":               "把 %s 换成了另一份副本",
	"switched to %s":                            "切换到了 %s",
	"cancelled":                                 "已取消",
	"only enabled mods can be moved":            "只能移动已启用的 Mod",
	"expected a number":                         "需要一个序号",
	"expected: <kind> <ref>":                    "应为：<类型> <引用>",
	"scan complete · %d changes · c shows them": "✓ 扫描完成 · %d 处变化 · 按 c 查看",
	"scan complete · nothing changed":           "扫描完成 · 没有变化",
	"scan failed":                               "扫描失败",
	// drag (status line while dragging)
	"move %s to number %d":   "把 %s 移到第 %d 个",
	"move %s to the end":     "把 %s 移到最后",
	"enable %s at number %d": "启用 %s，放在第 %d 个",
	"enable %s":              "启用 %s",
	"stage %s":               "暂存 %s",
	"disable %s":             "停用 %s",
	"take %s out of staging": "把 %s 移出暂存",
	// prompts and secondary screens
	"archo command: ":     "archo 命令：",
	"move %s to number: ": "把 %s 移到第几个：",
	"rule for %s (after|before|attach|incompatible <ref>): ": "为 %s 加规则（after|before|attach|incompatible <引用>）：",
	"sort — preview":                  "自动排序",
	"save to game — preview":          "保存到游戏",
	"switch environment":              "切换环境",
	"no environments (archo env add)": "没有环境（archo env add）",
	"%d–%d of %d":                     "第 %d–%d 行，共 %d 行",
	"mod":                             "Mod",
	// detail column
	"official": "官方", "workshop": "工坊", "local": "本地", "local copy": "本地副本",
	"supports": "支持", "number %d": "第 %d 个", "staged": "暂存中", "not enabled": "未启用",
	"no problems found":                                  "没有发现问题",
	"needs attention · ! shows what":                     "需要处理 · 按 ! 查看",
	"the game cannot load it":                            "游戏无法加载它",
	"About.xml is renamed: the game cannot load it":      "About.xml 被改了名，游戏无法加载它",
	"no longer subscribed: the game ignores this folder": "已取消订阅，游戏会忽略这个文件夹",
	"listed in ModsConfig.xml, but not installed":        "ModsConfig.xml 里有它，但没有安装",
	"technical information · i":                          "技术信息 · i 展开",
	"technical information":                              "技术信息",
	"no preview":                                         "无预览",
	// double press (06 §9)
	"press %s again: %s": "再按一次 %s：%s",
	"press L again to load it; your unsaved edits are replaced": "再按一次 L 载入；未保存的修改会被替换",
	"the outside change is loaded into the list":                "已把外部修改载入列表",
	// command palette (06 §8)
	"command palette": "命令面板", "command": "命令", "environment": "环境", "switch to %s": "切换到环境 %s",
	"nothing matches": "没有匹配的结果", "run": "执行", "close": "关闭",
	// 需要处理 and changes (06 §4.1, §7)
	"%d mods are out of order":     "%d 个 Mod 的顺序不对",
	"show which":                   "展开查看",
	"this list":                    "这个列表",
	"missing dependencies":         "缺少的前置",
	"conflicts":                    "冲突",
	"last run":                     "上次运行",
	"needs your answer":            "需要你确认",
	"%d to handle":                 "%d 项需要处理",
	"nothing to handle":            "没有需要处理的",
	"nothing needs your attention": "没有需要你处理的事",
	"checking the mods' files…":    "正在检查 Mod 的文件…",
	"the mods' files are not checked yet · A checks them":                                    "还没有检查 Mod 的文件 · 按 A 检查",
	"the files changed since the last check · A checks again":                                "上次检查后文件有变化 · 按 A 重新检查",
	"check the files again":                                                                  "重新检查文件",
	"Core and the DLCs must come first in the list":                                          "Core 和 DLC 要排在最前面",
	"%s should load after %s":                                                                "%s 应该排在 %s 之后",
	"the order requirements of %d mods contradict each other":                                "%d 个 Mod 的顺序要求互相矛盾",
	"%s is enabled twice":                                                                    "同一个 Mod 启用了两份：%s",
	"the list has %s, but it is not installed":                                               "列表里有 %s，但没有安装它",
	"%s is enabled, but the game cannot load it":                                             "%s 已启用，但游戏无法加载它",
	"%s needs %s, but %s is not installed":                                                   "%s 需要 %s，但没有安装 %s",
	"%s needs %s, but %s is not enabled":                                                     "%s 需要 %s，但 %s 没有启用",
	"part of %s does not take effect: %s, which it targets, is not enabled":                  "%s 的部分内容没有生效（它针对的 %s 没有启用）",
	"%s is incompatible with %s":                                                             "%s 与 %s 不兼容",
	"part of %s builds on a definition no enabled mod provides; a dependency may be missing": "%s 的部分内容继承的定义不存在，可能缺少前置 Mod",
	"%d mods do not list %s as supported":                                                    "%d 个 Mod 没有标明支持 %s",
	"%d mods do not list %s as supported (%d of them list %s)":                               "%d 个 Mod 没有标明支持 %s（其中 %d 个支持 %s）",
	"the list was saved for %s; the game resets it when it starts — saving once fixes it":    "列表是为 %s 保存的，游戏启动时会重置它；保存一次即可",
	"last run, %d errors could not be tied to a mod":                                         "上次运行时有 %d 次错误没能归到某个 Mod",
	"last run, %d warnings could not be tied to a mod":                                       "上次运行时有 %d 次警告没能归到某个 Mod",
	"last run, %s reported %d errors":                                                        "上次运行时 %s 报了 %d 次错",
	"last run, %s reported %d warnings":                                                      "上次运行时 %s 报了 %d 次警告",
	"earlier log · %s · %s":                                                                  "更早的日志 · %s · %s",
	"ModsConfig.xml was changed outside RimArcho":                                            "ModsConfig.xml 在 RimArcho 之外被修改了",
	"is %s the same mod as %s?":                                                              "%s 和 %s 是同一个 Mod 吗？",
	"same":                                                                                   "是同一个",
	"a copy":                                                                                 "是副本",
	"unrelated":                                                                              "无关",
	"%s and %s are one mod":                                                                  "%s 和 %s 已合并为同一个 Mod",
	"%s is a copy of %s":                                                                     "%s 记为 %s 的副本",
	"%s and %s are unrelated":                                                                "%s 和 %s 记为无关",
	"load it into the list":                                                                  "载入到列表",
	"ignore it":                                                                              "忽略",
	"load the outside change into the list?":                                                 "把外部修改载入列表？",
	"your unsaved edits are replaced by the list in ModsConfig.xml":                          "未保存的修改会被 ModsConfig.xml 里的列表替换",
	"the outside change is ignored (it stays in the history)":                                "已忽略外部修改（历史里仍保留）",
	"choose which to disable":                                                                "选择停用哪一个",
	"disable which one?":                                                                     "停用哪一个？",
	"choose which to keep":                                                                   "选择保留哪一份",
	"keep which copy?":                                                                       "保留哪一份？",
	"keep the one from %s":                                                                   "保留 %s 的那份",
	"kept one copy":                                                                          "已保留一份",
	"disable it":                                                                             "停用它",
	"remove it from the list":                                                                "从列表中移除",
	"open %s in the Workshop":                                                                "在工坊打开 %s",
	"look for it in the Workshop":                                                            "在工坊搜索它",
	"see what":                                                                               "查看",
	"see which":                                                                              "查看是哪些",
	"see it":                                                                                 "查看",
	"see the list":                                                                           "查看列表",
	"see the log and evidence":                                                               "查看日志与证据",
	"ignore the suggested one":                                                               "忽略建议的那一条",
	"that requirement is ignored from now on":                                                "这条顺序要求以后会被忽略",
	"nothing to ignore here":                                                                 "这里没有可忽略的",
	"order requirements that contradict each other":                                          "互相矛盾的顺序要求",
	"%s wants to load after %s":                                                              "%s 要求排在 %s 之后",
	"%s wants to load before %s":                                                             "%s 要求排在 %s 之前",
	"suggested to ignore (x)":                                                                "建议忽略（x）",
	"a removed mod":                                                                          "已移除的 Mod",
	"the log is read":                                                                        "已读取日志",
	"scan %s":                                                                                "%s 的扫描",
	"scan complete":                                                                          "扫描完成",
	"c shows them":                                                                           "按 c 查看",
	"nothing changed":                                                                        "没有变化",
	"%d mods updated":                                                                        "%d 个 Mod 有更新",
	"%d new mods":                                                                            "%d 个新 Mod",
	"%d mods gone":                                                                           "%d 个 Mod 不见了",
	"%d changed their package id":                                                            "%d 个改了包名",
	// full-screen sub-sections and history (06 §6.1, §6.3)
	"Defs": "Def", "full history": "完整历史",
	"sections": "跳到子区", "none": "无",
	"loads after %s (About.xml)":             "排在 %s 之后（About.xml）",
	"loads before %s (About.xml)":            "排在 %s 之前（About.xml）",
	"%s loads after %s (your rule)":          "%s 排在 %s 之后（你的规则）",
	"%s loads before %s (your rule)":         "%s 排在 %s 之前（你的规则）",
	"%s is incompatible with %s (your rule)": "%s 与 %s 不兼容（你的规则）",
	"%s follows %s (your rule)":              "%s 跟随 %s（你的规则）",
	"incompatible with %s (About.xml)":       "与 %s 不兼容（About.xml）",
	"… %d more · tab shows all":              "… 还有 %d 行 · tab 查看全部",
	"first seen":                             "首次发现", "updated": "更新", "moved": "移动了位置", "copied": "复制",
	"gone": "消失", "reappeared": "重新出现", "package id changed": "改了包名", "renamed": "改了名称",
	"author changed": "换了作者", "supported versions changed": "支持的版本变了",
	"dependencies changed": "依赖变了", "load order rules changed": "排序规则变了",
	// details: facts, requirements, description (06 §6.1, §6.4)
	"package": "包名", "version": "版本", "opens the page": "打开页面", "tags": "标签",
	"none · # adds": "无 · 按 # 添加", "alias": "别名", "note": "备注", "requires": "依赖",
	" (not enabled)": "（未启用）", " (not installed)": "（未安装）",
	"description": "简介", "loading…": "加载中…", "(empty)": "（空）",
	"description · About.xml":                  "简介 · About.xml",
	"description · Workshop page":              "简介 · 创意工坊页面",
	"fetching the Workshop page…":              "正在获取创意工坊页面…",
	"the Workshop page is fetched in a moment": "稍后获取创意工坊页面",
	"offline (workshop.fetch)":                 "离线（workshop.fetch）",
	"Workshop page: ":                          "创意工坊页面：",
	"… enter shows all":                        "… 按 enter 查看全部",
	"image":                                    "图片", "image loading… ": "图片加载中… ",
	"opened the Workshop page":   "已打开创意工坊页面",
	"not a workshop mod":         "不是创意工坊 Mod",
	"%d%% · ↑↓ PgUp PgDn scroll": "%d%% · ↑↓ PgUp PgDn 滚动",
	"Workshop page":              "创意工坊页面",
	"refresh":                    "刷新",
	"technical pages":            "技术页面",
	// tags (06 §5.6)
	"tags for %s (word adds, -word removes): ": "%s 的标签（词 = 添加，-词 = 移除）：",
	"added %s": "添加了 %s", "removed %s": "移除了 %s", "tags of %s: %s": "%s 的标签：%s",
	"has": "已有", "tab completes": "tab 补全", "no tags yet: type one": "还没有标签：直接输入一个",
	// Problems, Changes and Inspect screens (plain versions until V3/V4)
	"(%d–%d of %d, ↑/↓)":                              "（%d–%d / 共 %d，↑/↓）",
	"no problems":                                     "没有问题",
	"analysis is stale — A to re-analyze":             "分析结果已过期 — 按 A 重新分析",
	"not analyzed yet — A to analyze (child process)": "尚未分析 — 按 A 分析（子进程）",
	"analysis failed":                                 "分析失败",
	"severity: ":                                      "级别：",
	"problems %d/%d/%d":                               "问题 %d/%d/%d",
	"problems —":                                      "问题 —",
	"analysis done":                                   "分析完成",
	"[ignore expired]":                                "[忽略已失效]",
	"evidence: ":                                      "证据：",
	"error":                                           "错误",
	"warning":                                         "警告",
	"info":                                            "提示",
	"all":                                             "全部",
	"showing ignored":                                 "显示已忽略",
	"ignored %d (I shows)":                            "已忽略 %d（I 显示）",
	"no suggested command for this problem":           "此问题没有建议命令",
	"select single mods with the s filter to ignore them": "请用 s 筛选出单个 Mod 再忽略",
	"suggestion":                        "建议",
	"(not a command; open it yourself)": "（不是命令，请自行打开）",
	"run this command?":                 "运行这条命令？",
	"summary":                           "概要",
	"defs":                              "Def",
	"patches":                           "补丁",
	"problems":                          "问题",
	"rules":                             "规则",
	"history":                           "历史",
	"no Player.log found":               "未找到 Player.log",
	"analyze which log? (enter)":        "分析哪个日志？（enter）",
	"log analyzed: %d problems":         "日志分析完成：%d 个问题",
	"log analyzed: %d problems from a stale log, not counted": "日志分析完成：%d 个问题，来自旧日志，不计入",
	"log":                                  "日志",
	"current":                              "当前",
	"stale: written by %s, the game is %s": "旧日志：由 %s 写入，游戏为 %s",
	"stale: older than the Loadout's last change":                       "旧日志：早于列表最近一次改动",
	"%d error, %d warning from this log; not counted":                   "此日志 %d 个错误、%d 个警告；不计入",
	"%d enabled mods do not list %s":                                    "%d 个已启用的 Mod 没有标明支持 %s",
	" (%d list %s, %d older only)":                                      "（其中 %d 个支持 %s，%d 个只支持更早的版本）",
	"external ModsConfig.xml version ignored (it stays in loadout log)": "已忽略外部修改的 ModsConfig.xml（仍保留在 loadout log 中）",
	"… %d more": "… 另有 %d 项",
	"(semantic files unchanged: only other files changed)": "（语义文件未变，只有其他文件变化）",
	"(snapshot no longer stored)":                          "（快照已不在库中）",
	"(no file-level difference recorded for this event)":   "（此事件没有文件级差异记录）",
	"no changes":            "没有变化",
	"no scan yet (R scans)": "尚未扫描（R 扫描）",
	"changed outside RimArcho, kept as v%d — L load into the workspace · I ignore": "在 RimArcho 之外被修改，已存为 v%d — L 载入工作区 · I 忽略",
	"score": "相似度",
	"identity questions %d — s same · c copy · u unrelated":                "身份待确认 %d 项 — s 是同一个 · c 是副本 · u 无关",
	"%d bool prefixes from %d mods can skip methods other mods also patch": "%d 个返回 bool 的前缀（来自 %d 个 Mod）可能跳过其他 Mod 也在补丁的方法",
	helpText: helpTextZh,
}

const helpTextZh = `四列（06 §5）
  ↑↓ j k     移动光标         ←→ h l tab   切换列          [ ]   切换列表（窄终端）
  space      启用 / 停用 / 从暂存放回
  t          暂存 / 移出暂存          T     全部从暂存放回原位置
  J K        上移 / 下移              m     移到第几个
  S          自动排序（先看差异）     w     保存到游戏（先看差异）
  u ctrl+r   撤销 / 重做              L     启动游戏
  /          筛选这一列               v     多选
  enter      详情                     i     技术信息
  !          需要处理                 c     最近变化
  R          重新扫描                 E     切换环境
  :          运行 archo 命令          ctrl+k 命令面板
  ? 帮助   q 退出
鼠标：点击选中；在列之间拖动 = 启用、停用或暂存；在“已启用”内拖动 = 调整顺序；滚轮滚动`
