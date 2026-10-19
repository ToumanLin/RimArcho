package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/identity"
	"github.com/rimarcho/rimarcho/internal/store"
)

// The four columns (06 §5): not enabled | enabled | staging | detail.

type colKind int

const (
	cInactive colKind = iota
	cActive
	cStaging
)

func (k colKind) title() string {
	return [...]string{T("Disabled"), T("Enabled"), T("Staging")}[k]
}

// Row status, worst first wins (06 §3: one status icon at most).
const (
	statusNone = iota
	statusUnloadable
	statusWarn
	statusLog
	statusErr
)

// item is one row.
type item struct {
	id, ref string
	name    string
	lower   string // name, alias, author, package, folder: what / matches
	icon    string
	pos     int  // 1-based load position (enabled column)
	unsaved bool // changed since the last save
	dim     bool // cannot be loaded
	missing bool // a ModsConfig entry with no installed mod
}

type column struct {
	kind        colKind
	items       []item
	view        []int // indexes into items after the filter
	cursor, off int
	filter      string
}

func (c *column) cur() *item {
	if c.cursor < 0 || c.cursor >= len(c.view) {
		return nil
	}
	return &c.items[c.view[c.cursor]]
}

func (c *column) curID() string {
	if it := c.cur(); it != nil {
		return it.id
	}
	return ""
}

// refilter applies the filter and keeps the cursor on keepID when it is still shown.
func (c *column) refilter(keepID string) {
	c.view = c.view[:0]
	q := strings.ToLower(strings.TrimSpace(c.filter))
	for i := range c.items {
		if q == "" || words(q, c.items[i].lower) {
			c.view = append(c.view, i)
		}
	}
	// Nothing contains the text: letters in order in the name ("hsl" finds "Hair Style").
	if len(c.view) == 0 && q != "" && !strings.Contains(q, " ") {
		for i := range c.items {
			if subsequence(q, strings.ToLower(c.items[i].name)) {
				c.view = append(c.view, i)
			}
		}
	}
	c.cursor = min(c.cursor, max(len(c.view)-1, 0))
	if keepID != "" {
		for i, vi := range c.view {
			if c.items[vi].id == keepID {
				c.cursor = i
				break
			}
		}
	}
}

// words: every word of q is a substring of s (name, alias, author, package, folder).
func words(q, s string) bool {
	for _, w := range strings.Fields(q) {
		if !strings.Contains(s, w) {
			return false
		}
	}
	return true
}

// subsequence: the letters of q appear in s in order.
func subsequence(q, s string) bool {
	rs := []rune(s)
	i := 0
	for _, r := range q {
		for i < len(rs) && rs[i] != r {
			i++
		}
		if i == len(rs) {
			return false
		}
		i++
	}
	return true
}

func (c *column) scroll(rows int) {
	rows = max(rows, 1)
	if c.cursor < c.off {
		c.off = c.cursor
	}
	if c.cursor >= c.off+rows {
		c.off = c.cursor - rows + 1
	}
	c.off = max(0, min(c.off, max(len(c.view)-rows, 0)))
}

// ---- layout ----

const colGap = 3

type slot struct {
	kind colKind
	x, w int
}

type layout struct {
	slots            []slot
	detailX, detailW int // detailW 0: no detail column
	tabs             bool
}

// layout places the columns for the terminal width (06 §5.1): staging hides below 140
// (focusing it shows it in the first slot), the detail column below 110 (enter opens it
// full screen), and below 80 one column at a time with [ ] to switch.
func (m *model) layout() layout {
	w := m.w
	left := cInactive
	if m.focus == cStaging {
		left = cStaging
	}
	var l layout
	split := func(rest int, ratios ...int) []int {
		sum := 0
		for _, r := range ratios {
			sum += r
		}
		out := make([]int, len(ratios))
		used := 0
		for i, r := range ratios {
			out[i] = rest * r / sum
			used += out[i]
		}
		out[len(out)-1] += rest - used
		return out
	}
	switch {
	case w >= 140:
		l.detailW = max(40, w-105)
		ws := split(w-l.detailW-3*colGap, 30, 40, 26)
		l.slots = []slot{{cInactive, 0, ws[0]}, {cActive, ws[0] + colGap, ws[1]}, {cStaging, ws[0] + ws[1] + 2*colGap, ws[2]}}
		l.detailX = w - l.detailW
	case w >= 110:
		l.detailW = max(40, w-76)
		ws := split(w-l.detailW-2*colGap, 30, 40)
		l.slots = []slot{{left, 0, ws[0]}, {cActive, ws[0] + colGap, ws[1]}}
		l.detailX = w - l.detailW
	case w >= 80:
		ws := split(w-colGap, 30, 40)
		l.slots = []slot{{left, 0, ws[0]}, {cActive, ws[0] + colGap, ws[1]}}
	default:
		l.tabs = true
		l.slots = []slot{{m.focus, 0, w}}
	}
	return l
}

func (l layout) has(k colKind) bool {
	for _, s := range l.slots {
		if s.kind == k {
			return true
		}
	}
	return false
}

// rowsShown is the number of list rows: top bar, blank, titles … blank, status, hints.
func (m *model) rowsShown() int { return max(m.h-6, 1) }

// ---- building the rows ----

// reload rebuilds the columns from the workspace (synced with ModsConfig.xml) and staging.
func (m *model) reload() {
	keep := [3]string{}
	for k := range m.cols {
		keep[k] = m.cols[k].curID()
	}
	staged, st, err := m.app.Staged()
	m.st, m.loadErr = st, err
	for k := range m.cols {
		m.cols[k].kind = colKind(k)
		m.cols[k].items = m.cols[k].items[:0]
	}
	if err != nil {
		for k := range m.cols {
			m.cols[k].view = nil
		}
		return
	}
	m.envName, m.gameVer = st.Ix.Env.Name, st.Ix.Env.GameVersion
	chs := st.Changes()
	m.unsaved = len(chs)
	unsaved := map[string]bool{}
	for _, c := range chs {
		unsaved[c.Entry.Key()] = true
	}
	inList := map[string]bool{}
	m.missing = 0
	act := &m.cols[cActive]
	for i, e := range st.List {
		if e.Instance == "" {
			m.missing++
			act.items = append(act.items, item{pos: i + 1, ref: "pkg:" + e.Missing, name: e.Missing, lower: e.Missing, missing: true, unsaved: unsaved[e.Key()]})
			continue
		}
		inList[e.Instance] = true
		if mod := st.Ix.ByID[e.Instance]; mod != nil {
			it := modItem(st, mod)
			it.pos, it.unsaved = i+1, unsaved[e.Key()]
			act.items = append(act.items, it)
		}
	}
	isStaged := map[string]bool{}
	for _, s := range staged {
		isStaged[s.Instance] = true
		m.cols[cStaging].items = append(m.cols[cStaging].items, modItem(st, st.Ix.ByID[s.Instance]))
	}
	ina := &m.cols[cInactive]
	for _, mod := range st.Ix.Mods {
		if inList[mod.ID] || isStaged[mod.ID] || mod.Status != identity.Live {
			continue
		}
		ina.items = append(ina.items, modItem(st, mod))
	}
	sort.SliceStable(ina.items, func(i, j int) bool { return ina.items[i].lower < ina.items[j].lower })
	for id := range m.marked {
		if st.Ix.ByID[id] == nil {
			delete(m.marked, id)
		}
	}
	if qs, err := m.app.Store.Questions(st.Ix.Env.ID, false); err == nil {
		m.question = len(qs)
	}
	if sc, ss, _ := m.app.LastScan(st.Ix.Env.ID); sc != nil && ss != nil {
		m.changes = 0
		for _, n := range ss.Events {
			m.changes += n
		}
	}
	for k := range m.cols {
		m.cols[k].refilter(keep[k])
		m.cols[k].scroll(m.rowsShown())
	}
}

func modItem(st *app.LoadoutState, mod *store.Mod) item {
	it := item{id: mod.ID, ref: st.Ix.Ref(mod.ID), name: mod.DisplayName(), icon: sourceIcon(mod)}
	it.lower = strings.ToLower(it.name + " " + mod.Name + " " + mod.Authors + " " + mod.PackageID + " " + mod.Rel)
	it.dim = mod.Unloadable != "" || mod.Residue || !st.Reg.Known(mod.ID) || !st.Reg.Loadable(mod.ID)
	return it
}

// status is the row's status icon: problems from the last analysis, unloadable mods.
func (m *model) rowStatus(it *item) int {
	s := m.prob.status[it.id]
	if it.missing {
		s = statusErr
	}
	if s == statusNone && it.dim {
		s = statusUnloadable
	}
	return s
}

// ---- drawing ----

// columnLines draws the list columns and the detail column into rows 2…h-4.
func (m *model) columnLines(n int) []string {
	l := m.layout()
	out := make([]string, n)
	cells := make([][]string, len(l.slots))
	for i, s := range l.slots {
		cells[i] = m.drawColumn(s, n, l.tabs)
	}
	var detail []string
	if l.detailW > 0 {
		detail = m.detailLines(l.detailW, n)
	}
	for y := 0; y < n; y++ {
		var b strings.Builder
		x := 0
		for i, s := range l.slots {
			b.WriteString(strings.Repeat(" ", s.x-x))
			b.WriteString(cells[i][y])
			x = s.x + s.w
		}
		if l.detailW > 0 {
			b.WriteString(strings.Repeat(" ", l.detailX-x))
			if y < len(detail) {
				b.WriteString(detail[y])
			}
		}
		out[y] = b.String()
	}
	return out
}

// drawColumn returns n lines of exactly s.w cells: the title, then the rows.
func (m *model) drawColumn(s slot, n int, tabs bool) []string {
	c := &m.cols[s.kind]
	focused := m.focus == s.kind
	out := make([]string, 0, n)
	if tabs {
		out = append(out, m.tabStrip(s.w))
	} else {
		out = append(out, m.columnTitle(c, focused, s.w))
	}
	rows := n - 1
	c.scroll(rows)
	if len(c.view) == 0 {
		var msg string
		switch {
		case c.filter != "":
			msg = T("nothing matches · esc clears the filter")
		case c.kind == cStaging:
			msg = T("staging is empty · t puts the selected mod here")
		case c.kind == cActive:
			msg = T("no mod is enabled · space enables one")
		default:
			msg = T("every mod is enabled")
		}
		for _, l := range wrap(msg, s.w-2) {
			out = append(out, "  "+sDimS.Render(fit(l, s.w-2)))
		}
	}
	for i := c.off; i < len(c.view) && len(out) < n; i++ {
		out = append(out, m.drawRow(c, i, focused, s.w))
	}
	for len(out) < n {
		out = append(out, strings.Repeat(" ", s.w))
	}
	return out
}

func (m *model) columnTitle(c *column, focused bool, w int) string {
	t := fmt.Sprintf("%s %d", c.kind.title(), len(c.items))
	if c.filter != "" || (focused && m.editing) {
		t = fmt.Sprintf("%s %d/%d", c.kind.title(), len(c.view), len(c.items))
	}
	st := sDimS
	if focused {
		st = sAccent
	}
	title := st.Render(fit(t, min(width.StringWidth(t), w)))
	used := min(width.StringWidth(t), w)
	if c.filter != "" || (focused && m.editing) {
		f := "  " + ic.search + " " + c.filter
		if focused && m.editing {
			f += "_"
		}
		fw := min(width.StringWidth(f), w-used)
		title += sSecond.Render(fit(f, fw))
		used += fw
	}
	return title + strings.Repeat(" ", max(w-used, 0))
}

// tabStrip replaces the column titles below 80 columns: the three lists, focused one as a pill.
func (m *model) tabStrip(w int) string {
	var b strings.Builder
	used := 0
	for k := cInactive; k <= cStaging; k++ {
		t := fmt.Sprintf(" %s %d ", k.title(), len(m.cols[k].items))
		tw := width.StringWidth(t)
		if used+tw > w {
			break
		}
		if k == m.focus {
			b.WriteString(sPill.Render(t))
		} else {
			b.WriteString(sDimS.Render(t))
		}
		used += tw
	}
	hint := " [ ]"
	if used+width.StringWidth(hint) <= w {
		b.WriteString(sDimS.Render(hint))
		used += width.StringWidth(hint)
	}
	return b.String() + strings.Repeat(" ", max(w-used, 0))
}

// drawRow: mark, position (enabled column), source icon, name, unsaved dot, status icon at
// the right edge (06 §5.2). Exactly w cells.
func (m *model) drawRow(c *column, vi int, focused bool, w int) string {
	it := &c.items[c.view[vi]]
	isCur := vi == c.cursor
	mark := "  "
	if m.marked[it.id] && it.id != "" {
		mark = sAccent.Render("+ ")
	}
	if isCur && focused {
		mark = sAccent.Render(markSel + " ")
	}
	used := 2
	var b strings.Builder
	b.WriteString(mark)
	if c.kind == cActive {
		p := fmt.Sprintf("%4d  ", it.pos)
		b.WriteString(sDimS.Render(p))
		used += len(p)
	}
	icon := it.icon
	if icon == "" {
		icon = " "
	}
	b.WriteString(sDimS.Render(icon) + "  ")
	used += 3
	st := m.rowStatus(it)
	statusW := 2 // a space and the icon
	nameW := max(w-used-statusW, 1)
	name := it.name
	if it.missing {
		name += " " + T("(not installed)")
	}
	dot := ""
	if it.unsaved {
		dot = " " + ic.unsaved
	}
	nw := min(width.StringWidth(name), nameW-width.StringWidth(dot))
	if nw < 1 {
		nw, dot = nameW, ""
	}
	nameText := fit(name, nw)
	var ns = sText
	switch {
	case isCur && focused:
		ns = sSel
	case it.dim || it.missing:
		ns = sDimS
	case !focused && !isCur:
		ns = sDimS
	}
	b.WriteString(ns.Render(nameText))
	if dot != "" {
		b.WriteString(sAccent.Render(dot))
	}
	used += nw + width.StringWidth(dot)
	b.WriteString(strings.Repeat(" ", max(w-used-statusW, 0)))
	used = max(used, w-statusW)
	b.WriteString(" " + statusIcon(st))
	return b.String()
}

func statusIcon(s int) string {
	switch s {
	case statusErr:
		return sErrS.Render(ic.err)
	case statusLog:
		return sErrS.Render(ic.logErr)
	case statusWarn:
		return sWarnS.Render(ic.warn)
	case statusUnloadable:
		return sDimS.Render(ic.unloadable)
	}
	return " "
}

// fit truncates (with …) and pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return width.FillRight(width.Truncate(s, w, "…"), w)
}

// wrap breaks s into lines of at most w cells, at spaces when it can and anywhere in CJK
// text; a rune is never split.
func wrap(s string, w int) []string {
	if w < 1 {
		return nil
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		var line []rune
		lw := 0
		lastSpace := -1
		for _, r := range para {
			rw := width.RuneWidth(r)
			if lw+rw > w && len(line) > 0 {
				if lastSpace > 0 && r != ' ' {
					out = append(out, strings.TrimRight(string(line[:lastSpace]), " "))
					line = append([]rune{}, line[lastSpace+1:]...)
				} else {
					out = append(out, strings.TrimRight(string(line), " "))
					line = line[:0]
				}
				lastSpace = -1
				lw = width.StringWidth(string(line))
				if r == ' ' && len(line) == 0 {
					continue
				}
			}
			if r == ' ' {
				lastSpace = len(line)
			}
			line = append(line, r)
			lw += rw
		}
		out = append(out, strings.TrimRight(string(line), " "))
	}
	return out
}
