package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/patterns"
)

// PatternCard is what the error pattern library (03 §6.8) says about one log cluster, for
// the game log page's detail (06 §2.10): what this kind of error usually is, its common
// causes, a next step and the kinds of fix. It is community background from shared logs,
// not a conclusion about the player's list: the attribution above it stays RimArcho's own.
// Only clusters that match a pattern get one (fallback rules say nothing and get none).
type PatternCard struct {
	Ref string `json:"ref"` // pattern:g:<id> (the CLI's and MCP's reference)
	// Level is how much it matters, from the pattern's stage and its first fix: high (the
	// save), medium (loading, play), low (noise, or "usually safe to ignore"); "" unknown.
	// Impact says it in a sentence, LevelText in a word.
	Level     string   `json:"level,omitempty"`
	LevelText string   `json:"levelText,omitempty"`
	Impact    string   `json:"impact,omitempty"`
	Summary   string   `json:"summary,omitempty"` // "this kind of error is usually: …"
	Causes    []string `json:"causes,omitempty"`
	Next      string   `json:"next,omitempty"` // the first kind of fix, as a step
	Fixes     []string `json:"fixes,omitempty"`
	Seen      string   `json:"seen,omitempty"` // how often shared logs show it
	Gone      string   `json:"gone,omitempty"` // repeat uploads: went away after small list changes
	// Known: a known error of the cluster's source mod (a mod-specific pattern).
	Known string `json:"known,omitempty"`
}

// fixWords are the library's kinds of fix in the player's words.
var fixWords = map[string]string{
	"disable":           "disable the mod that errors",
	"enable-dependency": "enable the missing dependency",
	"reorder":           "change the load order",
	"update":            "update the mods involved",
	"local-fix":         "edit the mod's files (lock its version first)",
	"ignore":            "usually safe to ignore",
	"new-game":          "start a new save",
}

// PatternCardOf is the card of cluster id in the log at path (nil when it matches no
// pattern, or the log is not analysed).
func PatternCardOf(a *app.App, st *app.LoadoutState, path, id string) *PatternCard {
	if st == nil || path == "" || id == "" {
		return nil
	}
	r, err := a.AnalyzeLog(path)
	if err != nil {
		return nil
	}
	id = strings.TrimPrefix(strings.TrimPrefix(id, "log:"), "E")
	for _, lc := range r.Clusters {
		if strings.TrimPrefix(lc.ID, "E") == id {
			return patternCard(st, lc)
		}
	}
	return nil
}

func patternCard(st *app.LoadoutState, lc *app.LogCluster) *PatternCard {
	var src []string
	for _, at := range lc.Attr {
		if at.Role != app.RoleSource {
			continue
		}
		if m := st.Ix.ByID[at.Instance]; m != nil && m.PackageID != "" {
			src = append(src, strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam"))
		}
	}
	d := patterns.Load()
	m := d.MatchCluster(lc.Class, lc.Template, src)
	if m.Generic == nil && m.Mod == nil {
		return nil
	}
	c := &PatternCard{}
	if g := m.Generic; g != nil {
		c.Ref = g.Ref()
		c.Summary = Tf("this kind of error is usually: %s", T(g.Summary))
		for _, x := range g.Causes {
			c.Causes = append(c.Causes, T(x))
		}
		for i, f := range g.Fixes {
			w, ok := fixWords[f]
			if !ok {
				w = f
			}
			if i == 0 {
				c.Next = Tf("try first: %s", T(w))
			}
			c.Fixes = append(c.Fixes, T(w))
		}
		if len(c.Fixes) == 1 {
			c.Fixes = nil // the next step says it already
		}
		c.Level, c.Impact = severity(g)
		if c.Level != "" {
			c.LevelText = T("severity " + c.Level)
		}
		c.Seen = Tf("seen in %s of %s shared %s logs", patterns.Thousands(g.Logs), patterns.Thousands(d.Logs), d.Game)
		c.Gone = goneLine(g.Res)
	}
	if p := m.Mod; p != nil {
		if c.Ref == "" {
			c.Ref = p.Ref()
		}
		when := p.First
		if p.Last != "" && p.Last != p.First {
			when += " – " + p.Last
		}
		c.Known = Tf("%s has this error itself in %s shared logs (%s); a newer version may have fixed it",
			TokName(st, sourceTok(st, p.Mod)), patterns.Thousands(p.Logs), when)
	}
	return c
}

// severity grades a pattern for the player: what it does to the game, and how much that
// matters. A pattern whose first fix is "ignore" is low whatever its stage.
func severity(g *patterns.Pattern) (string, string) {
	if g.Stage == "noise" || len(g.Fixes) > 0 && g.Fixes[0] == "ignore" {
		return "low", T("usually does not affect the game")
	}
	switch g.Stage {
	case "save":
		return "high", T("touches the save: it may not load, or lose things")
	case "load":
		return "medium", T("an error while loading: what it names may be missing or not work")
	case "runtime":
		return "medium", T("an error during play: it may repeat, stutter or break a feature")
	}
	return "", ""
}

// sourceTok is the instance token of the enabled mod with packageId pkg (else pkg:<pkg>).
func sourceTok(st *app.LoadoutState, pkg string) string {
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil && strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam") == pkg {
			return "@{" + m.ID + "}"
		}
	}
	return "pkg:" + pkg
}

// goneLine: how often the error went away when the same player changed 1–3 mods and
// uploaded again, next to no change (fewer than 5 pairs say nothing).
func goneLine(r *patterns.Resolution) string {
	if r == nil || r.Pairs["small"] < 5 {
		return ""
	}
	pc := func(a, b int) int { return (100*a + b/2) / b }
	s := Tf("after changing 1–3 mods it went away in %d%% of %d repeat uploads", pc(r.Gone["small"], r.Pairs["small"]), r.Pairs["small"])
	if n := r.Pairs["none"]; n >= 5 {
		s += Tf(" (%d%% without any change)", pc(r.Gone["none"], n))
	}
	return s
}

// WithoutPatternBlock drops the CLI's pattern: and known-error: sections from a cluster's
// text (archo log --cluster): the interfaces show them as the pattern card instead.
func WithoutPatternBlock(lines []string) []string {
	var out []string
	skip := -1 // indent of the section being dropped
	for _, l := range lines {
		ind := len(l) - len(strings.TrimLeft(l, " "))
		if skip >= 0 && ind > skip {
			continue
		}
		skip = -1
		if t := strings.TrimLeft(l, " "); ind > 0 && (strings.HasPrefix(t, "pattern: ") || strings.HasPrefix(t, "known-error: ")) {
			skip = ind
			continue
		}
		out = append(out, l)
	}
	return out
}

// PatternCardArgv is the card of the cluster an `archo log … --cluster E<n>` command shows
// (--file, --prev or the current Player.log), for the TUI's technical page; nil otherwise.
func PatternCardArgv(a *app.App, st *app.LoadoutState, argv []string) *PatternCard {
	if st == nil || len(argv) == 0 || argv[0] != "log" {
		return nil
	}
	path, id, prev := "", "", false
	for i := 1; i < len(argv); i++ {
		switch argv[i] {
		case "--file":
			if i+1 < len(argv) {
				path = argv[i+1]
				i++
			}
		case "--cluster":
			if i+1 < len(argv) {
				id = argv[i+1]
				i++
			}
		case "--prev":
			prev = true
		case "--text":
			return nil
		}
	}
	if path == "" {
		cur, p := a.LogPaths(st)
		path = cur
		if path == "" {
			path = p
		}
		if prev {
			path = p
		}
	}
	return PatternCardOf(a, st, path, id)
}

// Lines is the card as text, for the TUI: a heading that says it is community background,
// then the kind, causes, next step, kinds of fix and how often shared logs show it.
func (c *PatternCard) Lines() []string {
	if c == nil {
		return nil
	}
	out := []string{T("community background") + "  " + T("from shared logs, not a conclusion about your list")}
	add := func(k, v string) {
		if v != "" {
			out = append(out, "  "+k+"  "+v)
		}
	}
	if c.LevelText != "" {
		out = append(out, "  "+c.LevelText+"  "+c.Impact)
	}
	if c.Summary != "" {
		out = append(out, "  "+c.Summary)
	}
	add(T("common causes"), strings.Join(c.Causes, T("; ")))
	add(T("next step"), c.Next)
	add(T("kinds of fix"), strings.Join(c.Fixes, "  "))
	if c.Known != "" {
		out = append(out, "  "+c.Known)
	}
	if f := strings.Join(nonEmpty(c.Seen, c.Gone), "  "); f != "" {
		out = append(out, "  "+f)
	}
	return out
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}
