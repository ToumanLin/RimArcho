package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/bbcode"
	"github.com/ToumanLin/RimArcho/internal/view"
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

// Row status, worst first wins (06 §3: one status icon at most; view.Status*).
const (
	statusNone       = view.StatusNone
	statusUnloadable = view.StatusUnloadable
	statusWarn       = view.StatusWarn
	statusLog        = view.StatusLog
	statusErr        = view.StatusErr
)

// item is one row: the shared view.Item (08 §1.1) and the TUI's copies of its fields.
type item struct {
	v               view.Item
	id, ref         string
	name            string
	lower           string // sort key
	pkg, ws         string
	tags            []string
	kind            string // official, workshop, local, copy, git
	steamcmd, alias bool
	note            string
	icon            string
	pos             int  // 1-based load position (enabled column)
	unsaved         bool // changed since the last save
	dim             bool // cannot be loaded
	missing         bool // a ModsConfig entry with no installed mod
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
	// Stored widths (06 §12.1) replace the defaults; the detail column takes the rest.
	withDetail := func(kinds []colKind, defaults []int) {
		gaps := len(kinds) * colGap
		ws := m.fitWidths(kinds, defaults, w-gaps-minDetailW)
		x := 0
		for i, k := range kinds {
			l.slots = append(l.slots, slot{k, x, ws[i]})
			x += ws[i] + colGap
		}
		l.detailX, l.detailW = x, w-x
	}
	switch {
	case w >= 140:
		withDetail([]colKind{cInactive, cActive, cStaging}, split(w-max(40, w-105)-3*colGap, 30, 40, 26))
	case w >= 110:
		withDetail([]colKind{left, cActive}, split(w-max(40, w-76)-2*colGap, 30, 40))
	case w >= 80:
		// No detail column: the two lists share the width in proportion to their widths.
		def := split(w-colGap, 30, 40)
		ws := split(w-colGap, m.colWant(left, def[0]), m.colWant(cActive, def[1]))
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
	b, err := view.BuildBoard(m.app)
	st := b.St
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
	m.envName, m.gameVer = b.EnvName, b.GameVersion
	m.loadColPrefs(st.Ix.Env.ID)
	m.unsaved, m.missing = b.Unsaved, b.Missing
	for k := range b.Cols {
		for _, v := range b.Cols[k] {
			m.cols[k].items = append(m.cols[k].items, fromView(st, v))
		}
	}
	for id := range m.marked {
		if st.Ix.ByID[id] == nil {
			delete(m.marked, id)
		}
	}
	m.question = b.Questions
	if b.LastEvents != nil {
		m.changes, m.lastEvents = b.Changes, b.LastEvents
	}
	for k := range m.cols {
		m.refilterCol(colKind(k), keep[k])
	}
	m.loadHistory()
}

// fromView copies a shared row into the TUI's row and adds the source icon.
func fromView(st *app.LoadoutState, v view.Item) item {
	it := item{v: v, id: v.ID, ref: v.Ref, name: v.Name, lower: v.Lower, pos: v.Pos, unsaved: v.Unsaved, dim: v.Dim, missing: v.Missing}
	it.pkg, it.ws, it.tags, it.note, it.alias = v.Package, v.Workshop, v.Tags, v.Note, v.Alias
	it.kind, it.steamcmd = v.Kind, v.SteamCMD
	if mod := st.Ix.ByID[v.ID]; mod != nil {
		it.icon = sourceIcon(mod)
	}
	return it
}

// rowStatus is the row's status icon: problems from the last analysis, unloadable mods.
func (m *model) rowStatus(it *item) int { return view.RowStatus(&it.v, m.prob.status) }

// ---- drawing ----

// columnLines draws the list columns and the detail column into rows 2…h-4, and returns
// the detail's image placements.
func (m *model) columnLines(n int) ([]string, []placement) {
	l := m.layout()
	out := make([]string, n)
	cells := make([][]string, len(l.slots))
	for i, s := range l.slots {
		cells[i] = m.drawColumn(s, n, l.tabs)
	}
	var detail []string
	var ps []placement
	if l.detailW > 0 {
		detail, ps = m.detailLines(l.detailW, n, l.detailX)
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
	return out, ps
}

// drawColumn returns n lines of exactly s.w cells: the title, then the rows.
func (m *model) drawColumn(s slot, n int, tabs bool) []string {
	c := &m.cols[s.kind]
	focused := m.focus == s.kind && !m.detailFocus // area 4 has the focus: no list is bright (06 §13.3)
	out := make([]string, 0, n)
	if tabs {
		out = append(out, m.tabStrip(s.w))
	} else {
		out = append(out, m.columnTitle(c, focused, s.w))
	}
	rows := n - 1
	c.scroll(rows)
	rm := make([]int, 0, n)
	rm = append(rm, -1) // the title
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
			rm = append(rm, -1)
		}
		if c.kind == cStaging && m.historyShown() && len(out) < n-1 {
			out, rm = append(out, ""), append(rm, -1)
			for _, l := range m.histLines(s.w, n-len(out)) {
				out, rm = append(out, l), append(rm, -1)
			}
		}
	}
	if m.prefs.Wrap[c.kind] && !tabs {
		// Wrapped names: rows take several lines; keep the cursor's row in full view.
		for c.off < c.cursor {
			h := 0
			for i := c.off; i <= c.cursor; i++ {
				h += m.rowLines(c, i, s.w)
			}
			if h <= rows {
				break
			}
			c.off++
		}
	}
	for i := c.off; i < len(c.view) && len(out) < n; i++ {
		for _, line := range m.drawRowLines(c, i, focused, s.w, m.prefs.Wrap[c.kind] && !tabs) {
			if len(out) == n {
				break
			}
			out = append(out, line)
			rm = append(rm, i)
		}
	}
	for len(out) < n {
		out = append(out, strings.Repeat(" ", s.w))
		rm = append(rm, -1)
	}
	m.rowMap[s.kind] = rm
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
	num := fmt.Sprintf("%d ", int(c.kind)+1) // the area's number key (06 §13.3)
	title := sDimS.Render(num) + st.Render(fit(t, min(width.StringWidth(t), w-2)))
	used := 2 + min(width.StringWidth(t), w-2)
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
		t := fmt.Sprintf(" %d %s %d ", int(k)+1, k.title(), len(m.cols[k].items))
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
	hint := " [ ] 1 2 3"
	if used+width.StringWidth(hint) <= w {
		b.WriteString(sDimS.Render(hint))
		used += width.StringWidth(hint)
	}
	return b.String() + strings.Repeat(" ", max(w-used, 0))
}

// drawRow: mark, position (enabled column), source icon, name, unsaved dot, status icon at
// the right edge (06 §5.2). Exactly w cells.
func (m *model) drawRow(c *column, vi int, focused bool, w int) string {
	return m.drawRowLines(c, vi, focused, w, false)[0]
}

// drawRowLines draws a row; with wrap a long name continues on more lines, under the name
// (the position, icons and status are on the first line only, 06 §12.1).
func (m *model) drawRowLines(c *column, vi int, focused bool, w int, wrapName bool) []string {
	it := &c.items[c.view[vi]]
	isCur := vi == c.cursor
	// The focused area's cursor row has a faint background, so one place on the screen is
	// the brightest (06 §13.3). Every piece of the row carries it.
	bg := func(st lipgloss.Style) lipgloss.Style { return st }
	if isCur && focused {
		bg = func(st lipgloss.Style) lipgloss.Style { return st.Background(cRowBg) }
	}
	pad := func(n int) string { return bg(plainStyle).Render(strings.Repeat(" ", max(n, 0))) }
	mark := pad(2)
	selected := m.marked[it.id] && it.id != ""
	if isCur && focused {
		mark = bg(sAccent).Render(markSel + " ")
	}
	if selected {
		mark = bg(sAccent).Render("▌ ") // 06 §13.5
		if isCur && focused {
			mark = bg(sAccent).Render("▌" + markSel)
		}
	}
	used := 2
	var b strings.Builder
	b.WriteString(mark)
	if c.kind == cActive {
		p := fmt.Sprintf("%4d  ", it.pos)
		b.WriteString(bg(sDimS).Render(p))
		used += len(p)
	}
	icon := it.icon
	if icon == "" {
		icon = " "
	}
	b.WriteString(bg(sDimS).Render(icon) + pad(2))
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
	var more []string // wrapped lines after the first
	if wrapName && width.StringWidth(name) > nameW {
		if ls := wrap(name, nameW); len(ls) > 1 {
			name, more = ls[0], ls[1:]
		}
	}
	nw := min(width.StringWidth(name), nameW-width.StringWidth(dot))
	if more != nil {
		nw, dot = min(width.StringWidth(name), nameW), "" // the dot goes on the last line
	}
	if nw < 1 {
		nw, dot = nameW, ""
	}
	nameText := fit(name, nw)
	var ns = sText
	switch {
	case isCur && focused:
		ns = sSel
	case selected:
		ns = sText
	case it.dim || it.missing:
		ns = sDimS
	case !focused && !isCur:
		ns = sDimS
	}
	b.WriteString(bg(ns).Render(nameText))
	if dot != "" {
		b.WriteString(bg(sAccent).Render(dot))
	}
	used += nw + width.StringWidth(dot)
	// Tags: a faint suffix when the name leaves room (06 §5.6).
	if len(it.tags) > 0 {
		if avail := w - used - statusW; avail >= 6 {
			t := "  " + strings.Join(it.tags, " ")
			tw := min(width.StringWidth(t), avail)
			b.WriteString(bg(sDimS).Render(fit(t, tw)))
			used += tw
		}
	}
	b.WriteString(pad(w - used - statusW))
	b.WriteString(pad(1) + bg(statusStyle(st)).Render(statusGlyph(st)))
	lines := []string{b.String()}
	pre := w - nameW - statusW // the cells before the name
	for i, l := range more {
		lead := strings.Repeat(" ", pre)
		if selected {
			lead = sAccent.Render("▌") + strings.Repeat(" ", pre-1)
		}
		d := ""
		if i == len(more)-1 && it.unsaved && width.StringWidth(l)+2 <= nameW {
			d = " " + ic.unsaved
		}
		t := ns.Render(fit(l, width.StringWidth(l)))
		if d != "" {
			t += sAccent.Render(d)
		}
		lines = append(lines, lead+t+strings.Repeat(" ", max(w-pre-width.StringWidth(l)-width.StringWidth(d), 0)))
	}
	return lines
}

func statusIcon(s int) string {
	if s == statusNone {
		return " "
	}
	return statusStyle(s).Render(statusGlyph(s))
}

func statusGlyph(s int) string {
	switch s {
	case statusErr:
		return ic.err
	case statusLog:
		return ic.logErr
	case statusWarn:
		return ic.warn
	case statusUnloadable:
		return ic.unloadable
	}
	return " "
}

func statusStyle(s int) lipgloss.Style {
	switch s {
	case statusErr, statusLog:
		return sErrS
	case statusWarn:
		return sWarnS
	case statusUnloadable:
		return sDimS
	}
	return plainStyle
}

// fit truncates (with …) and pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return width.FillRight(width.Truncate(s, w, "…"), w)
}

// wrap breaks s into lines of at most w cells, the same way as the description (wrapSpans):
// at spaces, between CJK characters, never inside a rune or before closing punctuation,
// and a word longer than the line after a path separator when it has one. A line's
// leading spaces are kept on every line it wraps into (06 §6.1, T7).
func wrap(s string, w int) []string {
	if w < 1 {
		return nil
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		ind := min(len(para)-len(strings.TrimLeft(para, " ")), w/2)
		pad := strings.Repeat(" ", ind)
		for _, l := range wrapSpans([]bbcode.Span{{Text: para[ind:]}}, w-ind, plainStyle) {
			out = append(out, pad+strings.TrimRight(ansi.Strip(l), " "))
		}
	}
	return out
}

var plainStyle = lipgloss.NewStyle()
