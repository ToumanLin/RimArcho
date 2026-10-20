package view

import (
	"sort"
	"strings"
)

// The command palette (06 §8): commands (by their Chinese name or English alias), mods and
// the other environments. Ranking: prefix > substring > subsequence > description; ties go
// to commands, then mods, then environments, then the shorter name.

// Command is one palette command; each interface maps the ID to what it does.
type Command struct {
	ID   string `json:"id"`
	Zh   string `json:"-"`
	En   string `json:"alias"`
	Desc string `json:"desc"` // the TUI key and a few words, matched last
}

// Name is the command's name in the interface language.
func (c Command) Name() string {
	if lang == "zh" {
		return c.Zh
	}
	return c.En
}

// Commands of the main screen.
var Commands = []Command{
	{"sort", "自动排序", "sort", "S"},
	{"save", "保存到游戏", "save", "w ModsConfig.xml"},
	{"launch", "启动游戏", "launch", "L RimWorld"},
	{"undo", "撤销", "undo", "u"},
	{"redo", "重做", "redo", "ctrl+r"},
	{"needs", "需要处理", "needs attention", "! problems"},
	{"changes", "最近变化", "recent changes", "c history"},
	{"rescan", "重新扫描", "rescan", "R scan"},
	{"analyze", "重新检查文件", "check the files again", "A analyze"},
	{"log", "选择日志", "choose a log", "Player.log"},
	{"env", "切换环境", "switch environment", "E env"},
	{"restore-all", "全部从暂存放回", "put all back", "T staging"},
	{"stage-all", "启用列表全部放进暂存", "stage the whole enabled list", "T in the window"},
	{"filter", "筛选这一列", "filter", "/ search"},
	{"details", "详情", "details", "enter"},
	{"tech", "技术信息", "technical information", "i"},
	{"tags", "编辑标签", "tags", "#"},
	{"packs", "整合包", "packs", "build, play"},
	{"settings", "设置", "settings", "paths, language, data, caches"},
	{"cli", "运行 archo 命令", "command line", ": archo"},
	{"help", "帮助", "help", "?"},
	{"quit", "退出", "quit", "q"},
}

// Palette entry kinds, in tie order.
const (
	PalCommand = iota
	PalMod
	PalEnv
)

// PalEntry is one palette candidate.
type PalEntry struct {
	Kind  int    `json:"kind"`
	ID    string `json:"id"` // command id, environment name or instance id
	Name  string `json:"name"`
	Alias string `json:"alias,omitempty"`
	Desc  string `json:"desc,omitempty"`
	Score int    `json:"-"`
}

// PalScore ranks one candidate for q (lower case): 4 prefix, 3 substring, 2 subsequence,
// 1 description, 0 no match.
func PalScore(q string, e PalEntry) int {
	if q == "" {
		return 1
	}
	best := 0
	for _, f := range []string{e.Name, e.Alias} {
		f = strings.ToLower(f)
		if f == "" {
			continue
		}
		switch {
		case strings.HasPrefix(f, q):
			best = max(best, 4)
		case strings.Contains(f, q):
			best = max(best, 3)
		case Subsequence(q, f):
			best = max(best, 2)
		}
	}
	if best == 0 && strings.Contains(strings.ToLower(e.Desc), q) {
		best = 1
	}
	return best
}

// PalRank filters and orders the candidates for a query; an empty query lists the
// commands and environments only.
func PalRank(query string, all []PalEntry) []PalEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []PalEntry
	for _, e := range all {
		if q == "" && e.Kind == PalMod {
			continue
		}
		if s := PalScore(q, e); s > 0 {
			e.Score = s
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if q != "" && len(a.Name) != len(b.Name) {
			return len(a.Name) < len(b.Name)
		}
		return false
	})
	return out
}
