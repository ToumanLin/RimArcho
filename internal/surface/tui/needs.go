package tui

import (
	"errors"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// 需要处理 (06 §5). The items, their wording and their actions are view.BuildNeeds (06 §1);
// here each action gets its key and the screen is drawn.

type needKey struct {
	key, label string
	run        func(m *model) tea.Cmd
}

// need is one item of the screen: the shared item and its keys.
type need struct {
	group    int
	section  int            // the section it is shown in (view.SectionOf, 06 §2.4)
	evidence *view.Evidence // where the claim comes from: the chip under the item
	text     string
	keys     []needKey
	counted  bool     // in the top bar's number; false for the folded stale log
	mods     []string // instance ids: the first carries the row icon
	status   int      // statusErr, statusWarn or statusLog for the row icon
	dim      bool
	parts    []string
	fold     bool
}

func (m *model) toNeeds(items []view.Need) []need {
	out := make([]need, 0, len(items))
	for _, it := range items {
		n := need{group: it.Group, section: it.Section, evidence: it.Evidence, text: it.Text, counted: it.Counted, mods: it.Mods, status: it.Status, dim: it.Dim, parts: it.Parts, fold: it.Fold}
		for _, a := range it.Actions {
			n.keys = append(n.keys, m.actionKey(a, it.Text))
		}
		out = append(out, n)
	}
	// by section, stable: within a section the view's order (this run before the last run)
	sort.SliceStable(out, func(i, j int) bool { return out[i].section < out[j].section })
	return out
}

// evIcon is the evidence chip's icon (06 §7, §9.3).
func evIcon(kind string) string {
	switch kind {
	case view.EvDeclared:
		return ic.evDeclared
	case view.EvInferred:
		return ic.evInferred
	case view.EvLog:
		return ic.evLog
	case view.EvCommunity:
		return ic.evCommunity
	case view.EvRule:
		return ic.evRule
	}
	return markDot
}

// chipText is the evidence chip as compact text: "≣ declared by the mod · About.xml modDependencies".
func chipText(e *view.Evidence) string {
	if e == nil {
		return ""
	}
	s := evIcon(e.Kind) + " " + e.Label
	if e.Source != "" {
		s += " " + markDot + " " + e.Source
	}
	return s
}

// actionKey binds a shared action to its key (06 §5).
func (m *model) actionKey(a view.Action, text string) needKey {
	k := needKey{key: a.Key, label: a.Label}
	switch a.Kind {
	case view.ActSort:
		k.run = func(m *model) tea.Cmd { return m.listAction(view.ListSort) }
	case view.ActSave:
		k.run = func(m *model) tea.Cmd { return m.listAction(view.ListSave) }
	case view.ActOpenURL:
		k.run = func(m *model) tea.Cmd { return m.open(a.URL) }
	case view.ActView:
		k.run = func(m *model) tea.Cmd { return m.openInspect(a.Argv) }
	case view.ActList:
		k.run = func(m *model) tea.Cmd { m.openPanel(a.Title, strings.Join(a.Lines, "\n")); return nil }
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
				// Not undone with u: press twice (06 §6).
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
	case a.Undo:
		m.say(sayOK, done+" · "+T("u undoes"))
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

// listAction is S, w and L (06 §6): it acts at once and says what it did; sort adds "u
// undoes". A refusal says why and opens a panel with the commands that resolve it.
func (m *model) listAction(what string) tea.Cmd {
	o := view.Run(m.app, what)
	m.reload()
	m.detailRef = ""
	switch {
	case o.Refused != nil:
		m.say(sayFail, o.Text)
		if len(o.Refused) > 1 {
			m.openPanel(listTitle(what), strings.Join(o.Refused, "\n"))
		}
	case o.Undo:
		m.say(sayOK, o.Text+" · "+T("u undoes"))
	default:
		m.say(sayOK, o.Text)
	}
	return m.requestDetail()
}

// listPreview is the palette's 预览排序 / 预览写入 / 预览启动: the dry run, to read.
func (m *model) listPreview(what string) tea.Cmd {
	out, _ := m.run(what, "--dry-run")
	title := map[string]string{view.ListSort: "preview the sort", view.ListSave: "preview writing to the game", view.ListLaunch: "preview the launch"}[what]
	m.openPanel(T(title), out)
	return nil
}

// listTitle names a refused whole-list action.
func listTitle(what string) string {
	switch what {
	case view.ListSort:
		return T("sort")
	case view.ListSave:
		return T("write to the game")
	}
	return T("launch")
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
	sec := -1
	for i, it := range items {
		if it.section != sec {
			if sec >= 0 {
				rows = append(rows, row{"", -1})
			}
			sec = it.section
			rows = append(rows, row{"  " + m.sectionHead(sec, w-2), -1})
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
		if c := chipText(it.evidence); c != "" {
			rows = append(rows, row{"    " + sDimS.Render(fit(c, tw)), i})
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

// sectionHead is a section's heading: its title, its count and, for the run errors, which
// run they are from (06 §2.4).
func (m *model) sectionHead(sec, w int) string {
	n := 0
	for _, it := range m.prob.items {
		if it.section == sec && it.counted {
			n++
		}
	}
	title := T(view.SectionTitles[sec])
	head := sDimS.Render(title)
	if sec == view.SectionBreak {
		head = sErrS.Render(title)
	}
	used := width.StringWidth(title)
	add := func(s string, st lipgloss.Style) {
		s = width.Truncate(s, max(w-used, 0), "…")
		head += st.Render(s)
		used += width.StringWidth(s)
	}
	if n > 0 {
		add(" "+markDot+" ", sDimS)
		add(itoa(n), sText)
	}
	if sec == view.SectionRun {
		switch {
		case m.live != nil && m.live.Running && m.live.Pack != "":
			add(" "+markDot+" "+Tf("pack %s", m.live.Pack), sDimS)
		case m.live != nil && m.live.Running:
			add(" "+markDot+" "+T("this run"), sDimS)
		case m.prob.lastRun != "":
			add(" "+markDot+" "+m.prob.lastRun, sDimS)
		}
	}
	return head
}

// sectionStep moves the cursor to the first item of the previous / next section.
func (m *model) sectionStep(d int) {
	p := &m.prob
	if p.cursor >= len(p.items) {
		return
	}
	sec := p.items[p.cursor].section
	if d > 0 {
		for i, it := range p.items {
			if it.section > sec {
				p.cursor = i
				return
			}
		}
		return
	}
	prev := -1
	for _, it := range p.items {
		if it.section < sec {
			prev = it.section
		}
	}
	for i, it := range p.items {
		if it.section == prev {
			p.cursor = i
			return
		}
	}
}

func (m *model) needsKey(s string) tea.Cmd {
	p := &m.prob
	switch s {
	case "left":
		m.sectionStep(-1)
		return nil
	case "right":
		m.sectionStep(1)
		return nil
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
