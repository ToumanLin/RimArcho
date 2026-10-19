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
  :          运行 archo 命令          ? 帮助   q 退出
鼠标：点击选中；在列之间拖动 = 启用、停用或暂存；在“已启用”内拖动 = 调整顺序；滚轮滚动`
