package tui

import (
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rimarcho/rimarcho/internal/cmds"
)

// Inspect (06 §6): a full-screen panel showing any reference with the same text as the CLI
// command. enter follows the reference on the selected line, backspace goes back, and a
// mod has sub-tabs (tab / shift+tab): summary, Defs, Patches, problems, rules, history.

type inspectView struct {
	argv        []string
	lines       []string
	cursor, off int
	modRef      string // set for a mod: enables the sub-tabs
	sub         int
}

var modTabs = []string{"summary", "defs", "patches", "problems", "rules", "history"}

func modTabArgv(ref string, sub int) []string {
	switch modTabs[sub] {
	case "defs":
		return []string{"def", "--mod", ref}
	case "patches":
		return []string{"patches", "--mod", ref}
	case "problems":
		return []string{"problems", "--subject", ref, "--detail", "--no-analyze"}
	case "rules":
		return []string{"rule", "list"}
	case "history":
		return []string{"changes", ref}
	}
	return []string{"mod", ref}
}

// openInspect pushes a view for a command.
func (m *model) openInspect(argv []string) tea.Cmd {
	v := &inspectView{argv: argv}
	if len(argv) == 2 && argv[0] == "mod" {
		v.modRef = argv[1]
	}
	m.fillInspect(v)
	if m.insp != nil {
		m.inspHist = append(m.inspHist, m.insp)
	}
	m.insp = v
	return nil
}

func (m *model) fillInspect(v *inspectView) {
	argv := v.argv
	if v.modRef != "" {
		argv = modTabArgv(v.modRef, v.sub)
	}
	out, _ := cmds.Execute(m.app, argv)
	v.lines = strings.Split(strings.TrimRight(out, "\n"), "\n")
	v.cursor, v.off = 0, 0
}

var refRe = regexp.MustCompile(`(?:^|[\s(,\[])((?:L?#[a-z0-9]{4,})|(?:def:[A-Za-z0-9_.]+[/@][^\s,)]+)|(?:p:[0-9a-f]{6}))`)

// lineRef returns the first followable reference in a line and the command that shows it.
func lineRef(line string) []string {
	for _, mt := range refRe.FindAllStringSubmatch(line, -1) {
		r := mt[1]
		switch {
		case strings.HasPrefix(r, "def:"):
			return []string{"def", r}
		case strings.HasPrefix(r, "p:"):
			return []string{"problems", "show", r}
		case strings.HasPrefix(r, "L#"):
			return []string{"list", "--filter", "lineage:" + r[2:]}
		default:
			return []string{"mod", r}
		}
	}
	return nil
}

func (m *model) inspectKey(s string) tea.Cmd {
	v := m.insp
	page := max(m.h-4, 1)
	move := func(d int) {
		v.cursor = max(0, min(v.cursor+d, len(v.lines)-1))
		if v.cursor < v.off {
			v.off = v.cursor
		}
		if v.cursor >= v.off+page {
			v.off = v.cursor - page + 1
		}
	}
	switch s {
	case "esc", "backspace", "q":
		if n := len(m.inspHist); n > 0 {
			m.insp, m.inspHist = m.inspHist[n-1], m.inspHist[:n-1]
		} else {
			m.insp = nil
		}
	case "down", "j":
		move(1)
	case "up", "k":
		move(-1)
	case "pgdown", " ":
		move(page)
	case "pgup":
		move(-page)
	case "home", "g":
		move(-len(v.lines))
	case "end", "G":
		move(len(v.lines))
	case "tab", "shift+tab", "]", "[":
		if v.modRef != "" {
			d := 1
			if s == "shift+tab" || s == "[" {
				d = len(modTabs) - 1
			}
			v.sub = (v.sub + d) % len(modTabs)
			m.fillInspect(v)
		}
	case "enter":
		if v.cursor < len(v.lines) {
			if argv := lineRef(v.lines[v.cursor]); argv != nil {
				same := len(argv) == len(v.argv) && strings.Join(argv, " ") == strings.Join(v.argv, " ")
				if !same {
					return m.openInspect(argv)
				}
			}
		}
	}
	return nil
}

// inspectLines renders the view with its sub-tab bar.
func (m *model) inspectLines(n int) []string {
	v := m.insp
	var out []string
	if v.modRef != "" {
		var tabs []string
		for i, t := range modTabs {
			label := T(t)
			if i == v.sub {
				label = stTabOn.Render(label)
			}
			tabs = append(tabs, label)
		}
		out = append(out, strings.Join(tabs, "  ")+stDim.Render("   (tab)"))
	}
	for i := v.off; i < len(v.lines) && len(out) < n; i++ {
		l := v.lines[i]
		if i == v.cursor {
			l = stSel.Render(fit(l, m.w))
		}
		out = append(out, l)
	}
	return out
}
