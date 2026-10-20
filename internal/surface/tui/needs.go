package tui

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 需要处理 (06 §7). The items, their wording and their actions are view.BuildNeeds (08 §1.1);
// here each action gets its key and the screen is drawn.

// Groups, in screen order (06 §7.1).
const (
	gList     = view.GroupList
	gDeps     = view.GroupDeps
	gConflict = view.GroupConflict
	gLastRun  = view.GroupLastRun
	gConfirm  = view.GroupConfirm
)

type needKey struct {
	key, label string
	run        func(m *model) tea.Cmd
}

// need is one item of the screen: the shared item and its keys.
type need struct {
	group   int
	text    string
	keys    []needKey
	counted bool     // in the top bar's number; false for the folded stale log
	mods    []string // instance ids: the first carries the row icon
	status  int      // statusErr, statusWarn or statusLog for the row icon
	dim     bool
	parts   []string
	fold    bool
}

func (m *model) toNeeds(items []view.Need) []need {
	out := make([]need, 0, len(items))
	for _, it := range items {
		n := need{group: it.Group, text: it.Text, counted: it.Counted, mods: it.Mods, status: it.Status, dim: it.Dim, parts: it.Parts, fold: it.Fold}
		for _, a := range it.Actions {
			n.keys = append(n.keys, m.actionKey(a, it.Text))
		}
		out = append(out, n)
	}
	return out
}

// actionKey binds a shared action to its key (06 §7.2).
func (m *model) actionKey(a view.Action, text string) needKey {
	k := needKey{key: a.Key, label: a.Label}
	switch a.Kind {
	case view.ActSort:
		k.run = func(m *model) tea.Cmd { return m.sortPreview() }
	case view.ActSave:
		k.run = func(m *model) tea.Cmd { return m.savePreview() }
	case view.ActOpenURL:
		k.run = func(m *model) tea.Cmd { return m.open(a.URL) }
	case view.ActView:
		k.run = func(m *model) tea.Cmd { return m.openInspect(a.Argv) }
	case view.ActList:
		k.run = func(m *model) tea.Cmd { m.openPanel(a.Title, strings.Join(a.Lines, "\n"), nil); return nil }
	case view.ActFold:
		k.run = func(m *model) tea.Cmd {
			p := &m.prob
			if p.open == nil {
				p.open = map[string]bool{}
			}
			p.open[text] = !p.open[text]
			return nil
		}
	case view.ActChooseDisable:
		k.run = func(m *model) tea.Cmd {
			p := &panel{title: a.Title}
			for _, o := range a.Options {
				p.choices = append(p.choices, o.Label)
			}
			p.onPick = func(m *model, v string) tea.Cmd {
				for i, o := range a.Options {
					if o.Label == v {
						m.doAction(a, i)
					}
				}
				return nil
			}
			m.panel = p
			return nil
		}
	default:
		k.run = func(m *model) tea.Cmd {
			if a.Confirm != "" {
				// Not undone with u: press twice (06 §9).
				return m.arm(a.Key, a.Confirm, func(m *model) tea.Cmd { m.doAction(a, 0); return nil })
			}
			m.doAction(a, 0)
			return nil
		}
	}
	return k
}

// doAction runs a data action (view.Do) and shows what it did.
func (m *model) doAction(a view.Action, option int) {
	done, err := view.Do(m.app, a, option)
	m.reload()
	switch {
	case errors.Is(err, view.ErrNothing):
		m.say(sayInfo, done)
	case err != nil:
		m.say(sayFail, m.humanize(err.Error()))
	default:
		m.say(sayOK, done)
	}
	m.loadProblems()
}

func (m *model) open(u string) tea.Cmd {
	open := m.opts.OpenURL
	if open == nil {
		open = openURL
	}
	if err := open(u); err != nil {
		m.say(sayFail, err.Error())
	} else {
		m.say(sayOK, T("opened the Workshop page"))
	}
	return nil
}

// sortPreview and savePreview are the S and w of the main screen.
func (m *model) sortPreview() tea.Cmd {
	out, code := m.run("sort", "--dry-run")
	var confirm []string
	if code == cmds.ExitOK && !strings.HasPrefix(out, "sort: 0 moved") {
		confirm = []string{"sort"}
	}
	m.openPanel(T("sort — preview"), out, confirm)
	return nil
}

func (m *model) savePreview() tea.Cmd {
	out, code := m.run("save", "--dry-run")
	var confirm []string
	if code == cmds.ExitOK {
		confirm = []string{"save"}
	}
	m.openPanel(T("save to game — preview"), out, confirm)
	return nil
}

// ---- the screen ----

// needLines draws 需要处理: a count line, then the groups with their items; the selected
// item shows its keys.
func (m *model) needLines(n int) []string {
	p := &m.prob
	w := m.w
	items := p.items
	head := Tf("%d to handle", p.need)
	if p.need == 0 {
		head = T("nothing to handle")
	}
	switch {
	case m.analyzing:
		head += "   " + T("checking the mods' files…")
	case p.never:
		head += "   " + T("the mods' files are not checked yet · A checks them")
	case p.stale:
		head += "   " + T("the files changed since the last check · A checks again")
	}
	out := []string{"  " + sText.Render(fit(head, w-2)), ""}
	if len(items) == 0 {
		return append(out, "  "+sOK.Render(markOK)+" "+sSecond.Render(fit(T("nothing needs your attention"), w-4)))
	}
	p.cursor = max(0, min(p.cursor, len(items)-1))
	type row struct {
		text string
		item int
	}
	var rows []row
	grp := -1
	for i, it := range items {
		if it.group != grp {
			if grp >= 0 {
				rows = append(rows, row{"", -1})
			}
			grp = it.group
			rows = append(rows, row{"  " + sDimS.Render(fit(T(view.GroupTitles[grp]), w-2)), -1})
		}
		icon := statusIcon(it.status)
		if it.dim || it.status == statusNone {
			icon = " "
		}
		st := sText
		if it.dim {
			st = sDimS
		}
		sel := i == p.cursor
		var keys []string
		for _, k := range it.keys {
			keys = append(keys, k.key+" "+k.label)
		}
		hint := strings.Join(keys, "  "+markDot+"  ")
		tw := w - 6
		lines := wrap(it.text, tw)
		for j, l := range lines {
			lead := "    "
			if j == 0 {
				lead = "  " + icon + " "
				if sel {
					lead = "  " + sAccent.Render(markSel) + " "
				}
			}
			s := st
			if sel {
				s = sSel
			}
			rows = append(rows, row{lead + s.Render(fit(l, tw)), i})
		}
		if it.fold && p.open[it.text] {
			for _, part := range it.parts {
				for j, l := range wrap(part, tw-2) {
					lead := "    " + sDimS.Render(markDot) + " "
					if j > 0 {
						lead = "      "
					}
					rows = append(rows, row{lead + sSecond.Render(fit(l, tw-2)), i})
				}
			}
		}
		if sel && hint != "" {
			for _, l := range wrap(hint, tw) {
				rows = append(rows, row{"    " + sSecond.Render(fit(l, tw)), i})
			}
		}
	}
	// keep the selected item in view
	first, last := -1, 0
	for i, r := range rows {
		if r.item == p.cursor {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	first = max(first, 0)
	page := max(n-len(out), 1)
	if first < p.off {
		p.off = max(first-1, 0)
	}
	if last >= p.off+page {
		p.off = last - page + 1
	}
	for i := p.off; i < len(rows) && len(out) < n; i++ {
		out = append(out, rows[i].text)
	}
	return out
}

func (m *model) needsKey(s string) tea.Cmd {
	p := &m.prob
	switch s {
	case "up", "k":
		p.cursor = max(p.cursor-1, 0)
		return nil
	case "down", "j":
		p.cursor = min(p.cursor+1, max(len(p.items)-1, 0))
		return nil
	case "pgup":
		p.cursor = max(p.cursor-10, 0)
		return nil
	case "pgdown":
		p.cursor = min(p.cursor+10, max(len(p.items)-1, 0))
		return nil
	case "home", "g":
		p.cursor = 0
		return nil
	case "end", "G":
		p.cursor = max(len(p.items)-1, 0)
		return nil
	case "A":
		return m.startAnalyze()
	case "l":
		return m.chooseLog()
	}
	if p.cursor >= len(p.items) {
		return nil
	}
	if s == " " {
		s = "space"
	}
	for _, k := range p.items[p.cursor].keys {
		if k.key == s {
			return k.run(m)
		}
	}
	return nil
}
