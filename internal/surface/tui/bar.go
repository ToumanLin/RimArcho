package tui

import (
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The filter bar (06 §2.5, §2.9): one search line and the filters in force as chips, over
// 未启用 and 已启用 together in the editor (view.FilterBoard) and over the table in Mod 库存
// (view.BuildLibrary). `\` opens the groups, one line each: ↑↓ a group, ←→ a value, enter
// or space keeps it, x or - leaves it out, typing finds a value of the group ("更多…"),
// esc closes. The search line takes the words of §3.7; tab takes the grey completion.

// barState is a page's bar.
type barState struct {
	search  string
	filters []string // "key:value", "-key:value"
	typing  bool     // the search line has the keys
	open    bool     // the groups are shown and have the keys
	grp     int      // the group under the cursor
	val     int      // the value under the cursor (in the group's shown values)
	find    string   // typed to find a value of the group
	groups  []view.LibGroup
	conds   []view.LibCond
	ghost   string // the search's completion
}

// active: the bar narrows the rows.
func (b *barState) active() bool { return strings.TrimSpace(b.search) != "" || len(b.filters) > 0 }

// setFilter: on 1 keeps the value, -1 leaves it out, 0 drops it (as the GUI's left and right click).
func (b *barState) setFilter(f string, on int) {
	f = strings.TrimPrefix(f, "-")
	b.filters = slices.DeleteFunc(b.filters, func(x string) bool { return strings.EqualFold(strings.TrimPrefix(x, "-"), f) })
	switch on {
	case 1:
		b.filters = append(b.filters, f)
	case -1:
		b.filters = append(b.filters, "-"+f)
	}
}

// shownValues are a group's values to offer: those with rows or chosen, matching find.
func (b *barState) shownValues(g view.LibGroup) []view.LibChoice {
	var out []view.LibChoice
	q := strings.ToLower(b.find)
	for _, v := range g.Values {
		if v.Count == 0 && v.On == 0 {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(v.Label), q) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// barLines is how many body lines the bar takes on the editor.
func (m *model) barLines() int {
	if m.tab != tabList || m.full {
		return 0
	}
	return m.bar.lineCount()
}

func (b *barState) lineCount() int {
	n := 1
	if b.open {
		n += len(b.groups)
	}
	return n
}

// ---- the editor ----

// refilterBar applies the bar to 未启用 and 已启用 (the cursors stay on their mods).
func (m *model) refilterBar() {
	for _, k := range []colKind{cInactive, cActive} {
		m.refilterCol(k, m.cols[k].curID())
	}
}

// barRows filters the editor's board, with the groups when the bar is open.
func (m *model) barResult() *view.BarFilter {
	if m.board == nil {
		return nil
	}
	b := &m.bar
	key := strings.Join(append([]string{b.search, strconv.Itoa(m.board.seq)}, b.filters...), "\x00")
	if b.open {
		key += "\x00open"
	}
	if m.barCache != nil && m.barKey == key {
		return m.barCache
	}
	f := view.MatchFacts{Status: m.prob.status}
	if view.BarNeedsContent(b.search, b.filters, b.open) && m.st != nil {
		c, err := m.content.Get(m.app, m.st)
		if err != nil {
			m.say(sayFail, err.Error())
		}
		f.Content = c
	}
	mm := ""
	if m.st != nil {
		mm = m.st.GameMM()
	}
	r := view.FilterBoard(m.board.b, b.search, b.filters, f, b.open, mm)
	b.groups, b.conds, b.ghost = nil, r.Conds, r.Complete
	for _, g := range r.Groups {
		if len(b.shownValues(g)) > 0 || b.find != "" {
			b.groups = append(b.groups, g) // a group with nothing to offer is left out
		}
	}
	m.barCache, m.barKey = r, key
	return r
}

// barChanged: the search or the filters changed: filter again from the top.
func (m *model) barChanged() tea.Cmd {
	if m.tab == tabLibrary {
		m.libr.cursor, m.libr.off = 0, 0
		m.loadLibrary()
		return nil
	}
	for _, k := range []colKind{cInactive, cActive} {
		m.cols[k].cursor, m.cols[k].off = 0, 0
	}
	m.barCache = nil
	m.refilterBar()
	m.clampBar()
	return m.requestDetail()
}

// curBar is the bar of the page shown.
func (m *model) curBar() *barState {
	if m.tab == tabLibrary {
		return &m.libr.bar
	}
	return &m.bar
}

// clampBar keeps the open bar's cursor on a group and a value it has.
func (m *model) clampBar() {
	b := m.curBar()
	b.grp = max(0, min(b.grp, len(b.groups)-1))
	if len(b.groups) == 0 {
		b.val = 0
		return
	}
	b.val = max(0, min(b.val, len(b.shownValues(b.groups[b.grp]))-1))
}

// toggleBar is `\`: open or close the groups.
func (m *model) toggleBar() tea.Cmd {
	b := m.curBar()
	b.open, b.typing, b.find = !b.open, false, ""
	if m.tab == tabLibrary {
		m.loadLibrary()
	} else {
		m.barCache = nil
		m.refilterBar()
	}
	m.clampBar()
	return nil
}

// barTypeKey edits the search line; the rows follow each key.
func (m *model) barTypeKey(k tea.KeyMsg) tea.Cmd {
	b := m.curBar()
	switch k.Type {
	case tea.KeyEnter:
		b.typing = false
		return nil
	case tea.KeyEsc:
		b.typing = false
		if b.search == "" {
			return nil
		}
		b.search = ""
	case tea.KeyTab:
		if b.ghost == "" {
			return nil
		}
		b.search += b.ghost
	case tea.KeyBackspace:
		if r := []rune(b.search); len(r) > 0 {
			b.search = string(r[:len(r)-1])
		} else if len(b.filters) > 0 {
			b.filters = b.filters[:len(b.filters)-1] // backspace on an empty line drops the last chip
		} else {
			return nil
		}
	case tea.KeyRunes, tea.KeySpace:
		b.search += string(k.Runes)
	case tea.KeyUp, tea.KeyDown:
		b.typing = false
		if k.Type == tea.KeyUp {
			return m.key(tea.KeyMsg{Type: tea.KeyUp})
		}
		return m.key(tea.KeyMsg{Type: tea.KeyDown})
	default:
		return nil
	}
	return m.barChanged()
}

// barOpenKey: the keys of the open bar.
func (m *model) barOpenKey(k tea.KeyMsg) tea.Cmd {
	b := m.curBar()
	s := k.String()
	if len(b.groups) == 0 {
		if s == "esc" || s == `\` {
			b.open = false
		}
		return nil
	}
	vals := b.shownValues(b.groups[b.grp])
	cur := func() *view.LibChoice {
		if b.val < len(vals) {
			return &vals[b.val]
		}
		return nil
	}
	switch s {
	case "esc":
		if b.find != "" {
			b.find = ""
			m.clampBar()
			return nil
		}
		b.open = false
		return m.toggleBarClosed()
	case `\`:
		b.open = false
		return m.toggleBarClosed()
	case "up":
		b.grp, b.find = max(b.grp-1, 0), ""
		m.clampBar()
	case "down":
		b.grp, b.find = min(b.grp+1, len(b.groups)-1), ""
		m.clampBar()
	case "left":
		b.val = max(b.val-1, 0)
	case "right":
		b.val = min(b.val+1, max(len(vals)-1, 0))
	case "home":
		b.val = 0
	case "end":
		b.val = max(len(vals)-1, 0)
	case "enter", " ":
		if v := cur(); v != nil {
			on := 1
			if v.On == 1 {
				on = 0
			}
			b.setFilter(v.Filter, on)
			keep := v.Filter
			cmd := m.barChanged()
			m.keepBarValue(keep)
			return cmd
		}
	case "x", "-":
		if v := cur(); v != nil {
			on := -1
			if v.On == -1 {
				on = 0
			}
			b.setFilter(v.Filter, on)
			keep := v.Filter
			cmd := m.barChanged()
			m.keepBarValue(keep)
			return cmd
		}
	case "/", "ctrl+f":
		b.open, b.typing = false, true
		return m.toggleBarClosed()
	case "backspace":
		if r := []rune(b.find); len(r) > 0 {
			b.find = string(r[:len(r)-1])
			m.clampBar()
		}
	default:
		if k.Type == tea.KeyRunes {
			b.find += string(k.Runes)
			b.val = 0
			m.clampBar()
		}
	}
	return nil
}

// keepBarValue puts the open bar's cursor back on a value after the groups were rebuilt.
func (m *model) keepBarValue(f string) {
	b := m.curBar()
	key, _, _ := strings.Cut(strings.TrimPrefix(f, "-"), ":")
	for i, g := range b.groups {
		if g.Key == key {
			b.grp = i // the groups may have changed around it
		}
	}
	if b.grp >= len(b.groups) {
		return
	}
	for i, v := range b.shownValues(b.groups[b.grp]) {
		if strings.EqualFold(v.Filter, strings.TrimPrefix(f, "-")) {
			b.val = i
			return
		}
	}
}

// toggleBarClosed: the groups closed; the rows no longer need them.
func (m *model) toggleBarClosed() tea.Cmd {
	m.curBar().find = ""
	if m.tab == tabLibrary {
		m.loadLibrary()
	} else {
		m.barCache = nil
		m.refilterBar()
	}
	return nil
}

// ---- drawing ----

// barView draws the bar w wide: the search line with its chips and the toggle, then the
// groups when open.
func (m *model) barView(w int) []string {
	b := m.curBar()
	var line strings.Builder
	used := 0
	add := func(s string, cells int) {
		line.WriteString(s)
		used += cells
	}
	if m.tab == tabLibrary {
		add(sDimS.Render("1 "), 2) // Mod 库存's first area (06 §3.5)
	}
	add(sDimS.Render(ic.search+" "), 2)
	switch {
	case b.typing:
		t := b.search
		g := b.ghost
		if g == "" {
			g = "▌"
		}
		tw := min(width.StringWidth(t), max(w/2, 10))
		add(sText.Render(fit(t, tw))+sDimS.Render(g), tw+width.StringWidth(g))
	case b.search != "":
		tw := min(width.StringWidth(b.search), max(w/2, 10))
		add(sSecond.Render(fit(b.search, tw)), tw)
	default:
		t := T("search: words, tag:, author:, type:, has:, version:, -not") + "  /"
		if m.tab == tabLibrary {
			t = T("search: words, tag:, state:, source:, used:, -not") + "  /"
		}
		tw := min(width.StringWidth(t), max(w-24, 10))
		add(sDimS.Render(fit(t, tw)), tw)
	}
	toggle := Tf("%s filter", `\`)
	if n := len(b.conds); n > 0 {
		toggle += " " + itoa(n)
	}
	tgw := width.StringWidth(toggle)
	for _, c := range b.conds {
		chip := c.Label
		if c.Exclude {
			chip = "-" + chip
		}
		chip = "[" + chip + "]"
		cw := width.StringWidth(chip) + 2
		if used+cw+tgw+2 > w {
			break
		}
		st := sAccent
		if c.Exclude {
			st = sDimS
		}
		add("  "+st.Render(chip), cw)
	}
	pad := max(w-used-tgw, 1)
	ts := sDimS
	if b.open {
		ts = sAccent
	}
	out := []string{line.String() + strings.Repeat(" ", pad) + ts.Render(toggle)}
	if !b.open {
		return out
	}
	titleW := 0
	for _, g := range b.groups {
		titleW = max(titleW, width.StringWidth(g.Title))
	}
	for gi, g := range b.groups {
		var l strings.Builder
		lu := 0
		here := gi == b.grp
		ts := sDimS
		if here {
			ts = sAccent
		}
		mark := "  "
		if here {
			mark = sAccent.Render(markSel + " ")
		}
		l.WriteString(mark + ts.Render(fit(g.Title, titleW)) + "  ")
		lu += 2 + titleW + 2
		vals := b.shownValues(g)
		// a window of values that keeps the cursor's one in view
		start := 0
		if here {
			for start < b.val {
				cells := 0
				for i := start; i <= b.val && i < len(vals); i++ {
					cells += width.StringWidth(valText(vals[i])) + 2
				}
				if lu+cells+10 <= w {
					break
				}
				start++
			}
		}
		if start > 0 {
			l.WriteString(sDimS.Render("… "))
			lu += 2
		}
		more := false
		for i := start; i < len(vals); i++ {
			t := valText(vals[i])
			cw := width.StringWidth(t) + 2
			if lu+cw+8 > w {
				more = true
				break
			}
			st := sSecond
			switch {
			case vals[i].On == 1:
				st = sAccent
			case vals[i].On == -1:
				st = sDimS
			case vals[i].Cur:
				st = sText
			}
			if here && i == b.val {
				st = sSel
			}
			l.WriteString(st.Render(t) + "  ")
			lu += cw
		}
		switch {
		case here && b.find != "":
			l.WriteString(sText.Render(b.find + "▌"))
		case more || (here && len(vals) == 0):
			l.WriteString(sDimS.Render(T("more…")))
		}
		out = append(out, l.String())
	}
	return out
}

// valText: "美化 48", "-本地 37" (left out), "1.6 当前游戏 388".
func valText(v view.LibChoice) string {
	t := v.Label
	if v.On == -1 {
		t = "-" + t
	}
	if v.Cur {
		t += " (" + T("this game") + ")"
	}
	return t + " " + itoa(v.Count)
}

func itoa(n int) string { return strconv.Itoa(n) }
