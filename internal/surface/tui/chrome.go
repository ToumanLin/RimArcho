package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The top of the screen (16 §2.2, §9.1): the navigation row (the GUI's top navigation:
// the pages of shift+1–8), the context row (the page's sub-pages or conclusion on the
// left, its page actions on the right), then a blank line.

// bodyTop is the number of rows above the body.
const bodyTop = 3

// navState caches the navigation entries (view.Nav, the GUI's sidebar model) and what the
// last drawn top rows hold, for clicks.
type navState struct {
	key   string
	items []view.NavItem // the shift+1–8 entries in order; Saves, with no key, is left out
	hits  []navHit
}

// navHit is a clickable span of the top rows.
type navHit struct {
	row, x0, x1 int
	page        int    // 1–8: a navigation entry
	sub         string // a sub-entry's page ID
}

// navItems returns the navigation entries, rebuilt when the game, the open list or the
// current list changed; planChanged and a language change clear the key.
func (m *model) navItems() []view.NavItem {
	key := m.envName + "\x00" + itoa(int(m.launchesID))
	if m.st != nil && m.st.Plan != nil {
		key += "\x00" + itoa(int(m.st.Plan.ID))
	}
	if m.nav.items != nil && m.nav.key == key {
		return m.nav.items
	}
	var plans []view.PlanRow
	if m.loadErr == nil {
		plans, _ = view.PlanRows(m.app)
	}
	var out []view.NavItem
	for _, g := range view.Nav(0, plans, nil) {
		for _, it := range g.Items {
			if it.Key == "" {
				continue
			}
			if it.ID == view.PagePacks {
				it.Kids = nil // packs are a text panel in the TUI: no pack pages to switch to
			}
			out = append(out, it)
		}
	}
	m.nav.items, m.nav.key = out, key
	return out
}

// navAt is the navigation entry (1–8, 0 for none) and sub-entry the screen shows.
func (m *model) navAt() (int, string) {
	if p := m.panel; p != nil {
		return p.nav, p.navSub // settings, tasks, command output: no entry
	}
	switch m.tab {
	case tabHome:
		return 1, ""
	case tabProblems:
		return 2, ""
	case tabLibrary:
		return 6, view.PageLibrary
	case tabChanges:
		return 6, view.PageChanges
	case tabSubscribe:
		return 6, view.PageSubscribe
	case tabSearch:
		return 6, view.PageSearch
	}
	if m.st != nil && m.st.Plan != nil {
		return 4, view.PlanPage(m.st.Plan.ID)
	}
	return 4, ""
}

// topBar is the navigation row: the entries with their shift digit, the current one as a
// pill and Can it run's count in amber; the game on the right. When the names do not fit,
// the other entries keep only their digit.
func (m *model) topBar() string {
	at, _ := m.navAt()
	items := m.navItems()
	need := m.needCount()
	envText := T("no game")
	if m.envName != "" {
		envText = m.envName
		if mm := env.MajorMinor(m.gameVer); mm != "" {
			envText += " " + mm
		}
	}
	rw := width.StringWidth(envText)
	right := sSecond.Render(envText)
	var hits []navHit
	build := func(names bool) (string, int) {
		var b strings.Builder
		x := 2
		b.WriteString("  ")
		hits = hits[:0]
		for i, it := range items {
			n := i + 1
			digit := itoa(n)
			badge := ""
			if it.ID == view.PageNeeds && need > 0 {
				badge = itoa(need)
			}
			if i > 0 {
				gap := "  "
				if !names {
					gap = " "
				}
				b.WriteString(gap)
				x += len(gap)
			}
			x0 := x
			switch {
			case n == at:
				b.WriteString(sPillDim.Render(" "+digit+" ") + sPill.Bold(true).Render(it.Label))
				x += 3 + width.StringWidth(it.Label)
				if badge != "" {
					b.WriteString(sPillWarn.Render(" " + badge))
					x += 1 + len(badge)
				}
				b.WriteString(sPill.Render(" "))
				x++
			case !names:
				st := sDimS
				if badge != "" {
					st = sWarnB // Can it run has items: its digit is amber
				}
				b.WriteString(st.Render(digit))
				x++
			default:
				b.WriteString(sDimS.Render(digit+" ") + sSecond.Render(it.Label))
				x += 2 + width.StringWidth(it.Label)
				if badge != "" {
					b.WriteString(sWarnB.Render(" " + badge))
					x += 1 + len(badge)
				}
			}
			hits = append(hits, navHit{row: 0, x0: x0, x1: x, page: n})
		}
		return b.String(), x
	}
	left, lw := build(true)
	if lw+2+rw+2 > m.w {
		left, lw = build(false)
	}
	m.nav.hits = append(m.nav.hits[:0], hits...)
	if lw+2+rw+2 > m.w {
		return left
	}
	return left + strings.Repeat(" ", m.w-lw-rw-2) + right + "  "
}

// contextRow is the second row: the current entry's sub-pages (the lists, Mod library) or
// the page's conclusion on the left; the page actions on the right (16 §2.2).
func (m *model) contextRow() string {
	right, rw := m.pageActions()
	room := m.w - 2
	if rw > 0 {
		room -= rw + 3
	}
	left, lw, inset := m.contextLeft(max(room, 10))
	pre := strings.Repeat(" ", inset)
	if rw == 0 || inset+lw+3+rw+2 > m.w {
		return pre + left
	}
	return pre + left + strings.Repeat(" ", m.w-inset-lw-rw-2) + right + "  "
}

// contextLeft: sub-page switches, a panel's title, a detail's place, or the conclusion;
// inset is the cells before it (sub-page switches start one cell early, so their names, not
// their padding, are on column 2).
func (m *model) contextLeft(w int) (string, int, int) {
	at, sub := m.navAt()
	plain := func(s string, st interface{ Render(...string) string }) (string, int, int) {
		s = width.Truncate(s, w, "…")
		return st.Render(s), width.StringWidth(s), 2
	}
	switch {
	case m.panel != nil && sub == "":
		return plain(m.panel.title, sBold)
	case m.insp != nil || m.full || m.tags != nil:
		// the tag bar takes the status line, where the position would be
		return plain(m.position(), sSecond)
	case m.tab == tabRules:
		return plain(T("load-order rules"), sBold)
	case m.tab == tabProblems:
		if lead, _ := m.needsLayout(); !lead {
			return "", 0, 2 // longer than the row: the body's first lines have it
		}
		return plain(m.needsHead(), sText)
	}
	if at <= 0 || at > len(m.navItems()) {
		return "", 0, 2
	}
	s, n := m.subTabs(m.navItems()[at-1].Kids, sub, w)
	return s, n, 1
}

// subTabs draws sub-entries as switches: the open one bold on the row tint; the list the
// game launches marked with ► (07 §1.3). Those that do not fit are counted as +N.
func (m *model) subTabs(kids []view.NavItem, cur string, w int) (string, int) {
	var b strings.Builder
	x, more := 0, 0
	for _, k := range kids {
		label := width.Truncate(k.Label, 28, "…")
		tw := 1 + width.StringWidth(label) + 1
		mark := ""
		if k.Current && strings.HasPrefix(k.ID, "plan:") && len(kids) > 2 {
			mark = ic.play + " "
		}
		need := tw + width.StringWidth(mark)
		if x > 0 {
			need++
		}
		if x+need > w-3 {
			more++
			continue
		}
		if x > 0 {
			b.WriteString(" ")
			x++
		}
		x0 := x
		on := k.ID == cur
		if on {
			b.WriteString(sRowBold.Render(" " + label + " "))
		} else {
			b.WriteString(sSecond.Render(" " + label + " "))
		}
		x += tw
		if mark != "" {
			st := sDimS
			if on {
				st = sDimS.Background(cRowBg)
			}
			b.WriteString(st.Render(mark))
			x += width.StringWidth(mark)
		}
		m.nav.hits = append(m.nav.hits, navHit{row: 1, x0: 1 + x0, x1: 1 + x, sub: k.ID})
	}
	if more > 0 {
		t := " +" + itoa(more)
		b.WriteString(sDimS.Render(t))
		x += len(t)
	}
	return b.String(), x
}

// pageActions are the page's own actions, quiet on the right of the context row; the
// hint line keeps the actions of the object under the cursor (16 §2.2).
func (m *model) pageActions() (string, int) {
	if m.panel != nil || m.insp != nil || m.full || m.pal != nil || m.tags != nil {
		return "", 0
	}
	var parts []string
	w := 0
	add := func(s string, cells int) {
		if len(parts) > 0 {
			w += 3
		}
		parts = append(parts, s)
		w += cells
	}
	key := func(k, label string) {
		add(sBold.Render(k)+" "+sSecond.Render(label), width.StringWidth(k)+1+width.StringWidth(label))
	}
	switch m.tab {
	case tabList:
		if m.launches != "" { // only open here: launching plays the current list (07 §1.3)
			t := Tf("launching plays %s", m.launches)
			add(sDimS.Render(t), width.StringWidth(t))
		}
		if m.st != nil && m.st.WS != nil && m.st.WS.Conflict != 0 && m.w >= 110 {
			t := Tf("an outside change is not used yet (v%d)", m.st.WS.Conflict)
			add(sWarnS.Render(t), width.StringWidth(t))
		}
		if m.unsaved > 0 {
			t := Tf("%d unsaved", m.unsaved)
			add(sAccent.Render(ic.unsaved)+" "+sSecond.Render(t), 2+width.StringWidth(t))
			key("w", T("save"))
		}
		if m.live != nil && m.live.Blocked != "" {
			// another environment's game runs: launching is refused until it exits (09 §1)
			add(sSecond.Render(m.live.Blocked), width.StringWidth(m.live.Blocked))
		} else {
			key("L", T("launch"))
		}
	case tabProblems:
		if _, acts := m.needsLayout(); acts {
			for _, p := range m.needsActions() {
				key(p[0], p[1])
			}
		}
	}
	return strings.Join(parts, "   "), w
}

// needsActions are Can it run's page actions (16 §2.4): F first, the page's main action.
func (m *model) needsActions() [][2]string {
	var ps [][2]string
	if n := view.AutoFixCount(m.prob.base); n > 0 {
		ps = append(ps, [2]string{"F", Tf("fix %d automatically", n)})
	}
	return append(ps, [2]string{"A", T("check the files again")}, [2]string{"l", T("choose a log")})
}

// needsLayout places Can it run's conclusion and page actions: both in the context row when
// they fit; else the conclusion there and the actions on the hint line; else (a conclusion
// longer than the row) the conclusion wraps at the top of the body, the actions in the row.
func (m *model) needsLayout() (leadInRow, actsInRow bool) {
	lw := width.StringWidth(m.needsHead())
	aw := 0
	for i, p := range m.needsActions() {
		if i > 0 {
			aw += 3
		}
		aw += width.StringWidth(p[0]) + 1 + width.StringWidth(p[1])
	}
	switch {
	case 2+lw+3+aw+2 <= m.w:
		return true, true
	case 2+lw+2 <= m.w:
		return true, false
	}
	return false, true
}

// areaHead is an area's heading in w cells: its number key faint in the cursor's slot, the
// name in bold secondary (bold accent when focused), the rest (a count, a size) faint
// (16 §7).
func areaHead(num, name, rest string, focused bool, w int) string {
	st := sHead
	if focused {
		st = sSel
	}
	lead := ""
	if num != "" {
		lead = num + " "
	}
	name = width.Truncate(name, max(w-len(lead), 0), "…")
	rest = width.Truncate(rest, max(w-len(lead)-width.StringWidth(name), 0), "…")
	used := len(lead) + width.StringWidth(name) + width.StringWidth(rest)
	return sDimS.Render(lead) + st.Render(name) + sDimS.Render(rest) + strings.Repeat(" ", max(w-used, 0))
}

// keyLines lays out key hints in w cells, indented by two: "key label" pairs 3 cells apart,
// wrapped between pairs, never between a key and its label; keys bold, labels secondary
// (16 §2.2 hints).
func keyLines(pairs [][2]string, w int) []string {
	var out []string
	var b strings.Builder
	used := 0
	for _, p := range pairs {
		cells := width.StringWidth(p[0]) + 1 + width.StringWidth(p[1])
		if used > 0 && used+3+cells > w {
			out = append(out, "  "+b.String())
			b.Reset()
			used = 0
		}
		if used > 0 {
			b.WriteString("   ")
			used += 3
		}
		b.WriteString(sBold.Render(p[0]) + " " + sSecond.Render(width.Truncate(p[1], max(w-used-width.StringWidth(p[0])-1, 1), "…")))
		used += cells
	}
	if used > 0 {
		out = append(out, "  "+b.String())
	}
	return out
}

// navClick: a click on the navigation row opens that page; on the context row, that
// sub-page.
func (m *model) navClick(x, y int) tea.Cmd {
	for _, h := range m.nav.hits {
		if h.row != y || x < h.x0 || x >= h.x1 {
			continue
		}
		if h.page > 0 {
			return m.goPage(h.page)
		}
		return m.openSub(h.sub)
	}
	return nil
}

// openSub opens a sub-entry: a list, all lists, or a page of Mod library.
func (m *model) openSub(id string) tea.Cmd {
	m.full, m.insp, m.panel = false, nil, nil
	switch id {
	case view.PagePlans:
		return m.choosePlan()
	case view.PageLibrary:
		return m.openLibrary()
	case view.PageChanges:
		return m.openChangesOf("", "")
	case view.PageSubscribe:
		return m.openAdd("")
	case view.PageSearch:
		return m.openSearch()
	case view.PageModSettings:
		return m.modSettingsPanel()
	}
	if p := view.PlanOfPage(id); p != 0 {
		if m.st != nil && m.st.Plan != nil && m.st.Plan.ID == p {
			m.tab = tabList
			return nil
		}
		return m.openPlan(p)
	}
	return nil
}
