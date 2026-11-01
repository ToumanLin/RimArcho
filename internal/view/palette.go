package view

import (
	"sort"
	"strings"
)

// The command palette (06 §3.5): commands (by their Chinese name or English alias), mods and
// the other environments. Ranking: prefix > substring > subsequence > description; ties go
// to commands, then mods, then environments, then the shorter name.

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
	{"changes", "@palette.changes", "subscriptions and updates", "c subscriptions updates history"},
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

// PalScore ranks one candidate for q (lower case): 4 prefix or suffix, 3 substring, 2
// subsequence, 1 description, 0 no match. A suffix counts as a prefix: a Chinese name ends
// in its head noun, the sort query matches both automatic sorting and load-order rules.
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
		case strings.HasPrefix(f, q), strings.HasSuffix(f, q):
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
