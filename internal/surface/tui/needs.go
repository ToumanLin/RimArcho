package tui

import (
	"errors"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// Can it run (06 §5). The items, their wording and their actions are view.BuildNeeds (06 §1);
// here each action gets its key and the screen is drawn.

type needKey struct {
	key, label string
	run        func(m *model) tea.Cmd
}

// need is one item of the screen: the shared item and its keys.
type need struct {
	phaseHeading string
	group        int
	section      int            // the section it is shown in (view.SectionOf, 06 §2.4)
	evidence     *view.Evidence // where the claim comes from: the chip under the item
	text         string
	keys         []needKey
	counted      bool     // in the top bar's number; false for the folded stale log
	mods         []string // instance ids: the first carries the row icon
	status       int      // statusErr, statusWarn or statusLog for the row icon
	dim          bool
	parts        []string
	fold         bool
	also         []string // what merged into it (view.Need.Also): faint lines under the text
}

func (m *model) toNeeds(items []view.Need) []need {
	out := make([]need, 0, len(items))
	for _, it := range items {
		// shown inside another item, or handled on another page (06 §2.4)
		if it.Into != "" || it.Section == view.SectionElsewhere {
			continue
		}
		n := need{group: it.Group, section: it.Section, phaseHeading: it.PhaseHeading, evidence: it.Evidence, text: it.Text, counted: it.Counted, mods: it.Mods, status: it.Status, dim: it.Dim, parts: it.Parts, fold: it.Fold, also: it.Also}
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
	case view.EvCorpus:
		return ic.evCorpus
	}
	return "—"
}

// chipText is the evidence chip as compact text: "≣ declared by the mod · About.xml modDependencies".
func chipText(e *view.Evidence) string {
	if e == nil {
		return ""
	}
	s := evIcon(e.Kind) + " " + e.Label
	if e.Source != "" {
		s += "  " + e.Source
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
	case view.ActLocateDirectory:
		k.run = func(m *model) tea.Cmd { return m.locateDirectory(a.Refs[0], a.Label) }
	case view.ActView:
		if len(a.Argv) >= 2 && a.Argv[0] == "plan" && a.Argv[1] == "cost" {
			k.run = func(m *model) tea.Cmd { return m.startMemoryEstimate(true) }
		} else {
			k.run = func(m *model) tea.Cmd { return m.openInspect(a.Argv) }
		}
	case view.ActProblem, view.ActGameLog, view.ActManageLists: // the TUI has no list management page: its CLI view
		k.run = func(m *model) tea.Cmd { return m.openInspect(a.Argv) }
	case view.ActList:
		k.run = func(m *model) tea.Cmd { m.openPanel(a.Title, strings.Join(a.Lines, "\n")); return nil }
	case view.ActVersionList:
		k.run = func(m *model) tea.Cmd { return m.openVersionWarnings(a) }
	case view.ActSubscribe: // add a subscription with the Workshop IDs: Steam or SteamCMD, the dry run, enter
		k.run = func(m *model) tea.Cmd { return m.openAdd(strings.Join(a.Refs, " ")) }
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
	case view.ActRepairLinks:
		k.run = func(m *model) tea.Cmd {
			m.openPanel(a.Label, strings.Join(a.Lines, "\n\n"))
			m.panel.onEnter = func(m *model) tea.Cmd { m.doAction(a, 0); return nil }
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
	if (a.Kind == view.ActVersionMute || a.Kind == view.ActVersionUnmute) && m.dd != nil && m.st != nil {
		if mod := m.st.Ix.ByID[m.dd.ID]; mod != nil {
			m.dd.Version, _ = view.VersionReason(m.app, m.st, mod)
		}
	}
	switch {
	case errors.Is(err, view.ErrNothing):
		m.say(sayInfo, done)
	case err != nil:
		m.say(sayFail, m.humanize(err.Error()))
	case a.Undo:
		m.say(sayOK, done+Tf(", %s", T("u undoes")))
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
	if what == view.ListLaunch {
		if c := m.currentLaunchMemory(); c != nil && c.Assessment.RAMSeverity == "severe" {
			return m.confirmMemoryLaunch(c)
		}
	}
	return m.doListAction(what)
}
func (m *model) doListAction(what string) tea.Cmd {
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
		m.say(sayOK, o.Text+Tf(", %s", T("u undoes")))
	default:
		m.say(sayOK, o.Text)
	}
	return m.requestDetail()
}

// listPreview is the palette's preview the sort / preview saving / preview the launch: the dry run, to read.
func (m *model) listPreview(what string) tea.Cmd {
	out, _ := m.run(what, "--dry-run")
	title := map[string]string{view.ListSort: "preview the sort", view.ListSave: "preview saving", view.ListLaunch: "preview the launch"}[what]
	m.openPanel(T(title), out)
	return nil
}

// listTitle names a refused whole-list action.
func listTitle(what string) string {
	switch what {
	case view.ListSort:
		return T("sort")
	case view.ListSave:
		return T("save")
	}
	return T("launch")
}

// ---- the screen ----

// needLines draws Can it run: a count line, then the groups with their items; the selected
// item shows its keys.
func (m *model) needLines(n int) []string {
	p := &m.prob
	w := m.w
	items := p.items
	// the window's lead sentence (view.NeedsLead), then the state of the check; A is on
	// the hint line
	before, runs, hints := 0, 0, false
	for _, it := range items {
		switch {
		case it.counted && it.section == view.SectionRun:
			runs++
		case it.counted:
			before++
		}
		hints = hints || it.status == statusHint
	}
	plan := ""
	if m.st != nil && m.st.Plan != nil {
		plan = m.st.Plan.Name
	}
	head := view.NeedsLead(plan, before, runs, p.never, hints)
	switch {
	case m.analyzing:
		head += "   " + T("checking the mods' files…")
	case p.stale:
		head += "   " + T("the files changed since the last check")
	}
	out := []string{"  " + sText.Render(fit(head, w-2)), ""}
	if len(items) == 0 {
		out = append(out, "  "+sOK.Render(markOK)+" "+sSecond.Render(fit(T("nothing needs your attention"), w-4)))
		if away := m.elsewhereLines(w); len(away) > 0 {
			out = append(append(out, ""), away...)
		}
		return out
	}
	p.cursor = max(0, min(p.cursor, len(items)-1))
	type row struct {
		text string
		item int
	}
	var rows []row
	sec := -1
	phase, group := "", -1
	for i, it := range items {
		if it.section != sec {
			if sec >= 0 {
				rows = append(rows, row{"", -1})
			}
			sec = it.section
			rows = append(rows, row{"  " + m.sectionHead(sec, w-2), -1})
		}
		// while the game runs, the last run's items follow this run's: say which run they are
		// from, or the same phase headings repeat over the same errors
		if it.group == view.GroupLastRun && group == view.GroupThisRun {
			label := T("last run")
			if m.prob.lastRun != "" {
				label += "  " + m.prob.lastRun
			}
			rows = append(rows, row{"    " + sDimS.Render(label), -1})
		}
		if it.phaseHeading != "" && (phase != it.phaseHeading || group != it.group) {
			rows = append(rows, row{"    " + sDimS.Render(it.phaseHeading), -1})
		}
		phase, group = it.phaseHeading, it.group
		icon := statusIcon(it.status)
		if it.dim && it.status != statusHint || it.status == statusNone {
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
		hint := strings.Join(keys, "   ")
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
		for _, a := range it.also {
			for _, l := range wrap(a, tw-2) {
				rows = append(rows, row{"    " + sDimS.Render("+ "+fit(l, tw-2)), i})
			}
		}
		if it.fold && p.open[it.text] {
			for _, part := range it.parts {
				for j, l := range wrap(part, tw-2) {
					lead := "    " + sDimS.Render("–") + " "
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
	// what this page leaves to another: faint lines at the end (06 §2.4)
	if away := m.elsewhereLines(w); len(away) > 0 {
		rows = append(rows, row{"", -1})
		for _, l := range away {
			rows = append(rows, row{l, -1})
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
	hintsOnly := true
	for _, it := range m.prob.items {
		if it.section == sec {
			if it.counted {
				n++
			}
			if it.status != statusHint {
				hintsOnly = false
			}
		}
	}
	title := T(view.SectionTitles[sec])
	live := sec == view.SectionRun && m.live != nil && m.live.Running
	switch {
	case live:
		title = T("errors this run") // the window's title while the game runs (06 §2.4)
	case hintsOnly && sec != view.SectionHint:
		title = T("suggestions") // only hints: not a red Will cause errors
	}
	// its number among the sections shown: 1–4 jump to it (06 §3.5)
	num := itoa(m.sectionNumber(sec)) + " "
	head := sDimS.Render(num) + sDimS.Render(title)
	switch {
	case sec == view.SectionBreak && !hintsOnly:
		head = sDimS.Render(num) + sErrS.Render(title)
	case sec == view.SectionHint:
		// worth knowing is RimArcho's own knowledge: the accent, not dimmed
		head = sDimS.Render(num) + sAccent.Render(title)
	}
	used := width.StringWidth(num + title)
	add := func(s string, st lipgloss.Style) {
		s = width.Truncate(s, max(w-used, 0), "…")
		head += st.Render(s)
		used += width.StringWidth(s)
	}
	if n > 0 {
		add("  ", sDimS)
		add(itoa(n), sText)
	}
	if sec == view.SectionRun {
		switch {
		case live && m.live.Pack != "":
			add("  "+Tf("pack %s", m.live.Pack), sDimS)
		case live: // the title says it: errors this run
		case m.prob.lastRun != "":
			add("  "+m.prob.lastRun, sDimS)
		}
	}
	return head
}

// elsewhereLines are the faint closing lines: what is handled on another page, and where.
func (m *model) elsewhereLines(w int) []string {
	var out []string
	for _, e := range m.prob.elsewhere {
		out = append(out, "  "+sDimS.Render(fit("→ "+e.Text, w-2)))
	}
	return out
}

// sectionNumber is a section's number among the sections shown (1 first).
func (m *model) sectionNumber(sec int) int {
	n, last := 0, -1
	for _, it := range m.prob.items {
		if it.section != last {
			n, last = n+1, it.section
		}
		if it.section == sec {
			return n
		}
	}
	return n
}

// sectionGo moves the cursor to the first item of the n-th section shown.
func (m *model) sectionGo(n int) {
	k, last := 0, -1
	for i, it := range m.prob.items {
		if it.section != last {
			k, last = k+1, it.section
			if k == n {
				m.prob.cursor = i
				return
			}
		}
	}
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

// autoFix is F, fix automatically (06 §2.4, §5): the required mods that are installed get enabled and
// the list is sorted again, one undo step.
func (m *model) autoFix() tea.Cmd {
	if view.AutoFixCount(m.prob.base) == 0 {
		m.say(sayInfo, T("nothing to fix automatically"))
		return nil
	}
	if m.verb(view.CompleteDependencies(m.app)) {
		m.prob.loaded = false
		m.say(sayOK, m.status+Tf(", %s", T("u undoes")))
		return m.ensureProblems()
	}
	return nil
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
	case "1", "2", "3", "4":
		m.sectionGo(int(s[0] - '0'))
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
	case "F":
		return m.autoFix()
	case "c": // subscriptions and updates, from any page (06 §3.5)
		return m.openChangesOf("", "")
	case "l":
		return m.chooseLog()
	}
	if s == " " {
		s = "space"
	}
	if p.cursor < len(p.items) {
		for _, k := range p.items[p.cursor].keys {
			if k.key == s {
				return k.run(m)
			}
		}
	}
	if s == "u" || s == "ctrl+z" { // undo what an item or F just did, as in the window
		m.act(T("undid the last change"), "undo")
		m.prob.loaded = false
		return m.ensureProblems()
	}
	return nil
}
