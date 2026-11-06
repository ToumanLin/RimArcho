package view

import (
	"sort"
	"strings"
	"unicode"
)

// The command palette (16 §3.5): commands (by their Chinese name or English alias), mods and
// the other environments. Ranking: prefix > substring > word starts > subsequence > word
// starts with typos > description; ties go to commands, then mods, then environments, then
// the shorter name.

// Command is one palette command; each interface maps the ID to what it does.
type Command struct {
	ID   string `json:"id"`
	Key  string `json:"-"`
	En   string `json:"alias"`
	Desc string `json:"desc"` // the TUI key and a few words, matched last
}

// Name is the command's name in the interface language.
func (c Command) Name() string {
	if lang := Lang(); lang != "en" {
		if name := localeExtra(lang, c.Key); name != c.Key {
			return name
		}
	}
	return c.En
}

// Commands of the main screen.
var Commands = []Command{
	{"sort", "@palette.sort", "sort", "S"},
	{"save", "@palette.save", "save", "w save ModsConfig.xml"},
	{"launch", "@palette.launch", "launch", "L RimWorld"},
	{"preview-sort", "@palette.preview-sort", "preview the sort", "S dry run"},
	{"preview-save", "@palette.preview-save", "preview saving", "w dry run ModsConfig.xml"},
	{"preview-launch", "@palette.preview-launch", "preview the launch", "L dry run"},
	{"undo", "@palette.undo", "undo", "u"},
	{"redo", "@palette.redo", "redo", "ctrl+r"},
	{"needs", "@palette.needs", "can it run", "@ shift+2 problems needs attention"},
	{"changes", "@palette.changes", "updates", "c subscriptions updates history"},
	{"library", "@palette.library", "mod library", "all mods, copies, sizes, filter, clean up"},
	{"subscribe", "@palette.subscribe", "add a subscription", "+ Git repository download page LoversLab Workshop SteamCMD"},
	{"rescan", "@palette.rescan", "rescan", "R scan"},
	{"analyze", "@palette.analyze", "check the files again", "A analyze"},
	{"log", "@palette.log", "game log", "l Player.log choose a log"},
	{"env", "@palette.env", "switch game", "E env environment"},
	{"plans", "@palette.plans", "lists", "P switch, new, rename, delete a list"},
	{"new-plan", "@palette.new-plan", "new list", "+ list copy import"},
	{"restore-all", "@palette.restore-all", "put all back", "T staging"},
	{"stage-all", "@palette.stage-all", "stage everything enabled", "T in the window"},
	{"import", "@palette.import", "import a list", "i RimSort RimCrow .rml share code collection"},
	{"export", "@palette.export", "export the list", "e RimArcho RimSort RimCrow .rml share code"},
	{"filter", "@palette.filter", "filter", "/ search"},
	{"details", "@palette.details", "details", "enter"},
	{"tech", "@palette.tech", "technical information", "i"},
	{"tags", "@palette.tags", "tags", "g"},
	{"packs", "@palette.packs", "packs", "build, play"},
	{"settings", "@palette.settings", "settings", "paths, language, data, caches"},
	{"cli", "@palette.cli", "command line", ": archo"},
	{"update", "@palette.update", "update RimArcho", "new version release notes install upgrade"},
	{"feedback", "@palette.feedback", "feedback", "GitHub issue report a bug suggestion"},
	{"uninstall", "@palette.uninstall", "uninstall RimArcho", "remove what RimArcho put on this machine"},
	{"textures", "@palette.textures", "texture optimisation", "convert mods' PNG textures to DDS"},
	{"search-files", "@palette.search-files", "search files", "grep rg Def patch C# source decompile"},
	{"mod-settings", "@palette.mod-settings", "mod settings", "Mod options configuration Config window values"},
	{"rules", "@palette.rules", "load-order rules", "r rule after before incompatible top bottom"},
	{"tasks", "@palette.tasks", "background tasks", "ctrl+j running waiting failed queue"},
	{"help", "@palette.help", "help", "?"},
	{"quit", "@palette.quit", "quit", "q"},
}

// Palette entry kinds, in tie order. A plan entry switches to that plan (its ID is
// the plan's).
const (
	PalCommand = iota
	PalMod
	PalEnv
	PalPlan
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

// PalScore ranks one candidate for q (lower case): 6 prefix or suffix, 5 substring, 4 word
// starts, 3 subsequence, 2 word starts with typos, 1 description, 0 no match. A suffix
// counts as a prefix: a Chinese name ends in its head noun, the sort query matches both
// automatic sorting and load-order rules. Word starts: every word of q begins a different
// word of the name, in any order, so "ad cu" finds Add Cube and "lib hug" finds HugsLib.
func PalScore(q string, e PalEntry) int {
	if q == "" {
		return 1
	}
	qw := palWords(q)
	best := 0
	for _, f := range []string{e.Name, e.Alias} {
		if f == "" || best == 6 {
			continue
		}
		lf := strings.ToLower(f)
		if strings.HasPrefix(lf, q) || strings.HasSuffix(lf, q) {
			best = 6
			continue
		}
		if strings.Contains(lf, q) {
			best = max(best, 5)
			continue
		}
		if best >= 4 {
			continue
		}
		fw := palWords(f)
		switch {
		case wordStarts(qw, fw, false):
			best = 4
		case Subsequence(q, lf):
			best = max(best, 3)
		case best < 2 && wordStarts(qw, fw, true):
			best = 2
		}
	}
	if best == 0 && (strings.Contains(strings.ToLower(e.Desc), q) || wordStarts(qw, palWords(e.Desc), false)) {
		best = 1
	}
	return best
}

// palWords splits s into lower-case words at spaces, punctuation and lower-to-upper case
// changes: "HugsLib" is hugs and lib.
func palWords(s string) [][]rune {
	buf := make([]rune, 0, len(s)) // one backing array; the words are slices of it
	var out [][]rune
	start := 0
	cut := func() {
		if len(buf) > start {
			out = append(out, buf[start:len(buf):len(buf)])
		}
		start = len(buf)
	}
	prevLower := false
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			cut()
			prevLower = false
			continue
		}
		if unicode.IsUpper(r) && prevLower {
			cut()
		}
		prevLower = unicode.IsLower(r)
		buf = append(buf, unicode.ToLower(r))
	}
	cut()
	return out
}

// palTypos is how many typos a query word may hold: none below 4 letters, then one, and
// two from 8 letters on.
func palTypos(n int) int {
	switch {
	case n < 4:
		return 0
	case n < 8:
		return 1
	}
	return 2
}

// wordStarts reports whether each query word begins a different name word. With typos, a
// query word may be palTypos edits (a swap of neighbours is one edit) away from the
// start of its name word.
func wordStarts(qw, fw [][]rune, typos bool) bool {
	if len(qw) == 0 || len(qw) > len(fw) {
		return false
	}
	if typos {
		some := false
		for _, w := range qw {
			some = some || palTypos(len(w)) > 0
		}
		if !some {
			return false // the same as without typos, which already failed
		}
	}
	used := make([]bool, len(fw))
	var assign func(i int) bool
	assign = func(i int) bool {
		if i == len(qw) {
			return true
		}
		for j, f := range fw {
			if used[j] {
				continue
			}
			var ok bool
			if typos {
				ok = prefixDist(qw[i], f, palTypos(len(qw[i])))
			} else {
				ok = len(f) >= len(qw[i]) && string(f[:len(qw[i])]) == string(qw[i])
			}
			if ok {
				used[j] = true
				if assign(i + 1) {
					return true
				}
				used[j] = false
			}
		}
		return false
	}
	return assign(0)
}

// prefixDist reports whether a is at most k edits (insert, delete, change, or swap two
// neighbours) away from some prefix of b.
func prefixDist(a, b []rune, k int) bool {
	if len(b) > len(a)+k {
		b = b[:len(a)+k]
	}
	m := len(b)
	var stack [3 * 24]int
	rows := stack[:]
	if 3*(m+1) > len(rows) {
		rows = make([]int, 3*(m+1))
	}
	prev2, prev, cur := rows[:m+1], rows[m+1:2*(m+1)], rows[2*(m+1):3*(m+1)]
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= m; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d = min(d, prev2[j-2]+1)
			}
			cur[j] = d
			rowMin = min(rowMin, d)
		}
		if rowMin > k {
			return false
		}
		prev2, prev, cur = prev, cur, prev2
	}
	for _, d := range prev {
		if d <= k {
			return true
		}
	}
	return false
}

// PlanEntries are the palette's "go to plan X" entries: every plan but the open one.
func PlanEntries(rows []PlanRow) []PalEntry {
	var out []PalEntry
	for _, r := range rows {
		if !r.Open {
			out = append(out, PalEntry{Kind: PalPlan, ID: r.Page, Name: Tf("list %s", r.Name), Alias: r.Name, Desc: "list"})
		}
	}
	return out
}

// PalRank filters and orders the candidates for a query; an empty query lists the
// commands, environments and plans only.
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
