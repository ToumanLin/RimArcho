package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// Picking (06 §12.2): first pick instances (selection only, nothing changes), then act on
// them. While anything is picked, the arrows move the picked instances: ↑↓ within the
// enabled list, ←→ to the neighbouring list. Every step is applied at once (the preview
// is the result), and the whole session is one undo step: enter keeps it, esc puts the
// lists back as they were. The picked set is m.marked; m.pick holds the session.

type pickState struct {
	col    colKind
	anchor string          // where a shift range starts
	base   map[string]bool // picks made with ctrl+space / ctrl+click, kept when a range changes
	acted  bool            // something was moved: shift+↑↓ moves by 10 from now on
	batch  *app.Batch
	// where the cursor was, for esc
	startFocus colKind
	startID    string
}

func (m *model) picking() bool { return m.pick != nil && len(m.marked) > 0 }

// beginPick opens a session in the focused column.
func (m *model) beginPick() {
	if m.pick != nil {
		return
	}
	b, err := m.app.BeginBatch()
	if err != nil {
		m.say(sayFail, err.Error())
		return
	}
	m.pick = &pickState{col: m.focus, base: map[string]bool{}, batch: b, startFocus: m.focus, startID: m.col().curID()}
	m.marked = map[string]bool{}
}

// endPick closes the session: commit keeps every step as one undo step, else they are undone.
func (m *model) endPick(commit bool) tea.Cmd {
	p := m.pick
	if p == nil {
		m.marked = map[string]bool{}
		return nil
	}
	first := m.firstPicked()
	n := len(m.marked)
	m.pick, m.marked = nil, map[string]bool{}
	if commit {
		if err := p.batch.Commit("pick"); err != nil {
			m.say(sayFail, err.Error())
		}
		m.reload()
		if first != "" {
			m.focusOn(first)
		}
		if p.acted {
			m.say(sayOK, Tf("done · %s · u undoes it all", plural(n, "mod")))
		}
	} else {
		if err := p.batch.Revert(); err != nil {
			m.say(sayFail, err.Error())
		}
		m.reload()
		m.focus = p.startFocus
		m.refilterCol(m.focus, p.startID)
		if p.acted {
			m.say(sayInfo, T("cancelled · the lists are as they were"))
		}
	}
	return m.requestDetail()
}

// pickedIDs: the picked instances in the order of their column.
func (m *model) pickedIDs() []string {
	var out []string
	if m.pick == nil {
		return nil
	}
	for _, it := range m.cols[m.pick.col].items {
		if it.id != "" && m.marked[it.id] {
			out = append(out, it.id)
		}
	}
	return out
}

func (m *model) firstPicked() string {
	if ids := m.pickedIDs(); len(ids) > 0 {
		return ids[0]
	}
	return ""
}

// focusOn puts the cursor on an instance in whichever column holds it.
func (m *model) focusOn(id string) {
	for k := range m.cols {
		for _, it := range m.cols[k].items {
			if it.id == id {
				m.focus = colKind(k)
				m.refilterCol(colKind(k), id)
				if m.cols[k].curID() != id { // hidden by the column's search
					m.cols[k].filter = ""
					m.refilterCol(colKind(k), id)
				}
				m.cols[k].scroll(m.rowsShown() - 1)
				return
			}
		}
	}
}

// pickKey handles a key while picking, or a picking key while nothing is picked; handled
// is false when the key is not about picking.
func (m *model) pickKey(s string) (tea.Cmd, bool) {
	c := m.col()
	switch s {
	case "shift+up", "shift+down":
		d := 1
		if s == "shift+up" {
			d = -1
		}
		if m.picking() && m.pick.acted {
			return m.pickMove(10 * d), true
		}
		if c.cur() == nil || c.curID() == "" {
			return nil, true
		}
		if !m.picking() {
			m.pick = nil
			m.beginPick()
		}
		if m.pick.anchor == "" {
			m.pick.anchor = c.curID()
		}
		m.moveCursor(d)
		m.pickRange()
		return m.requestDetail(), true
	case "ctrl+up", "ctrl+down":
		d := 1 // the cursor alone, picked or not
		if s == "ctrl+up" {
			d = -1
		}
		return m.moveCursor(d), true
	case "ctrl+@", "ctrl+ ", "v":
		return m.pickToggle(), true
	case " ":
		if !m.picking() {
			return m.pickToggle(), true
		}
		return m.pickToggle(), true
	}
	if !m.picking() {
		return nil, false
	}
	switch s {
	case "up", "k":
		return m.pickMove(-1), true
	case "down", "j":
		return m.pickMove(1), true
	case "left", "h":
		return m.pickShift(-1), true
	case "right", "l":
		return m.pickShift(1), true
	case "t":
		return m.pickStage(), true
	case "#":
		return m.tagKey(), true
	case "enter":
		return m.endPick(true), true
	case "esc":
		return m.endPick(false), true
	case "tab", "shift+tab", "[", "]":
		cmd := m.endPick(true)
		d := 1
		if s == "shift+tab" || s == "[" {
			d = -1
		}
		return tea.Batch(cmd, m.focusMove(d)), true
	case "?", "q", "ctrl+k", ":":
		return nil, false
	}
	m.say(sayInfo, T("picked: enter drops, esc cancels"))
	return nil, true
}

// pickToggle adds or removes the cursor's row (ctrl+space, space, v).
func (m *model) pickToggle() tea.Cmd {
	c := m.col()
	id := c.curID()
	if id == "" {
		return nil
	}
	if m.pick != nil && m.pick.col != m.focus {
		m.endPick(true)
	}
	m.beginPick()
	if m.pick == nil {
		return nil
	}
	if m.marked[id] {
		delete(m.marked, id)
		delete(m.pick.base, id)
	} else {
		m.marked[id] = true
		m.pick.base[id] = true
	}
	m.pick.anchor = id
	if len(m.marked) == 0 {
		return m.endPick(true)
	}
	return nil
}

// pickRange: the ctrl picks plus every row from the anchor to the cursor.
func (m *model) pickRange() {
	c := m.col()
	a, b := -1, c.cursor
	for i, vi := range c.view {
		if c.items[vi].id == m.pick.anchor {
			a = i
		}
	}
	if a < 0 {
		a = b
	}
	if a > b {
		a, b = b, a
	}
	m.marked = map[string]bool{}
	for id := range m.pick.base {
		m.marked[id] = true
	}
	for i := a; i <= b; i++ {
		if id := c.items[c.view[i]].id; id != "" {
			m.marked[id] = true
		}
	}
}

// pickMove moves the picked enabled mods by d places as one block (06 §12.2): a
// discontiguous pick first gathers at the place of its first instance.
func (m *model) pickMove(d int) tea.Cmd {
	if m.pick.col != cActive {
		m.say(sayInfo, T("↑↓ move mods within Enabled only"))
		return nil
	}
	ids := m.pickedIDs()
	first := len(m.st.List) + 1
	for _, id := range ids {
		first = min(first, m.st.Position(id))
	}
	rest := len(m.st.List) - len(ids)
	at := max(1, min(first+d, rest+1))
	m.pick.acted = true
	if _, err := m.app.MoveBlock(ids, at); err != nil {
		m.say(sayFail, err.Error())
		return nil
	}
	m.reload()
	m.focusOn(ids[0])
	return m.requestDetail()
}

// pickShift moves the picked instances to the list on the left (d -1) or right (d 1):
// not enabled ↔ enabled ↔ staging. Into "enabled" they go where the load-order
// constraints want them (the end when nothing constrains them).
func (m *model) pickShift(d int) tea.Cmd {
	ids := m.pickedIDs()
	if len(ids) == 0 {
		return nil
	}
	to := int(m.pick.col) + d
	if to < int(cInactive) || to > int(cStaging) {
		return nil
	}
	refs := make([]string, len(ids))
	for i, id := range ids {
		refs[i] = m.st.Ix.Ref(id)
	}
	var err error
	switch {
	case colKind(to) == cActive:
		at, e := m.app.SuggestAt(ids)
		if e != nil {
			err = e
			break
		}
		_, err = m.app.Enable(refs, app.Placement{At: at})
	case colKind(to) == cInactive: // from enabled
		_, err = m.app.Disable(refs)
	case colKind(to) == cStaging: // from enabled
		_, err = m.app.Stage(refs)
	}
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	if m.pick.col == cStaging { // out of staging into enabled
		m.app.Unstage(refs)
	}
	m.pick.acted = true
	m.pick.col = colKind(to)
	m.reload()
	m.focusOn(ids[0])
	return m.requestDetail()
}

// pickStage: t puts the picked instances into staging or takes them out, then the session
// ends there; the cursor stays in its column.
func (m *model) pickStage() tea.Cmd {
	ids := m.pickedIDs()
	refs := make([]string, len(ids))
	for i, id := range ids {
		refs[i] = m.st.Ix.Ref(id)
	}
	var err error
	if m.pick.col == cStaging {
		_, err = m.app.Unstage(refs)
	} else {
		_, err = m.app.Stage(refs)
	}
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	m.pick.acted = true
	col, keep := m.focus, m.col().curID()
	n := len(ids)
	p := m.pick
	m.pick, m.marked = nil, map[string]bool{}
	if err := p.batch.Commit("stage"); err != nil {
		m.say(sayFail, err.Error())
	}
	m.reload()
	m.focus = col
	m.refilterCol(col, keep)
	if col == cStaging {
		m.say(sayOK, Tf("took %s out of staging", plural(n, "mod")))
	} else {
		m.say(sayOK, Tf("staged %s", plural(n, "mod")))
	}
	return m.requestDetail()
}

// ---- the floating hint (06 §12.2) ----

// pickHint is the two lines shown beside the picked rows.
func (m *model) pickHint() []string {
	p := m.pick
	ids := m.pickedIDs()
	where := p.col.title()
	if p.col == cActive && m.st != nil {
		var pos []int
		for _, id := range ids {
			pos = append(pos, m.st.Position(id))
		}
		sort.Ints(pos)
		if len(pos) > 0 {
			if pos[len(pos)-1]-pos[0]+1 == len(pos) && len(pos) > 1 {
				where += fmt.Sprintf(" #%d–%d", pos[0], pos[len(pos)-1])
			} else {
				where += fmt.Sprintf(" #%d", pos[0])
				if len(pos) > 1 {
					where += " …"
				}
			}
		}
	}
	line1 := " " + Tf("picked %d", len(ids)) + " " + markDot + " " + where + " "
	var dests []string
	if p.col > cInactive {
		dests = append(dests, (p.col - 1).title())
	}
	if p.col < cStaging {
		dests = append(dests, (p.col + 1).title())
	}
	tLabel := T("stage")
	if p.col == cStaging {
		tLabel = T("unstage")
	}
	updown := sText.Inherit(sPill).Render("↑↓") + sSecond.Inherit(sPill).Render(" "+T("position"))
	if p.col != cActive {
		updown = sDimS.Inherit(sPill).Render("↑↓ " + T("position"))
	}
	key := func(k, l string) string {
		return sText.Inherit(sPill).Render(k) + sSecond.Inherit(sPill).Render(" "+l)
	}
	gap := sPill.Render("   ")
	line2 := sPill.Render(" ") + updown + gap + key("←→", T("move to")+" "+strings.Join(dests, " / ")) + gap +
		key("#", T("tags")) + gap + key("t", tLabel) + gap + key("enter", T("drop")) + gap + key("esc", T("cancel")) + sPill.Render(" ")
	return []string{sPill.Render(line1), line2}
}

// overlayHint draws the hint into the column lines near the picked rows, not over them.
func (m *model) overlayHint(out []string, l layout) {
	if !m.picking() || len(out) == 0 {
		return
	}
	var sl *slot
	for i := range l.slots {
		if l.slots[i].kind == m.pick.col {
			sl = &l.slots[i]
		}
	}
	if sl == nil {
		return
	}
	first, last := -1, -1
	for y, vi := range m.rowMap[m.pick.col] {
		c := &m.cols[m.pick.col]
		if vi >= 0 && vi < len(c.view) && m.marked[c.items[c.view[vi]].id] {
			if first < 0 {
				first = y
			}
			last = y
		}
	}
	hint := m.pickHint()
	if m.tags != nil {
		hint = append(hint, m.tagBarLine(max(m.w-sl.x-3, 20)))
	}
	y := last + 1
	switch {
	case first < 0: // scrolled away: at the bottom of the column
		y = len(out) - len(hint)
	case last+len(hint) >= len(out):
		y = max(first-len(hint), 1)
	}
	x := sl.x + 2
	for i, h := range hint {
		if y+i < 0 || y+i >= len(out) {
			continue
		}
		hw := min(ansi.StringWidthWc(h), m.w-x)
		if hw <= 0 {
			continue
		}
		h = ansi.TruncateWc(h, hw, "")
		line := out[y+i]
		if lw := ansi.StringWidthWc(line); lw < m.w {
			line += strings.Repeat(" ", m.w-lw)
		}
		out[y+i] = cutCells(line, 0, x) + h + cutCells(line, x+hw, m.w)
	}
}

// cutCells returns cells [left, right) of a styled line. ansi.CutWc (x/ansi v0.10.1) cuts
// the left side with TruncateWc instead of TruncateLeftWc and returns the line's start.
func cutCells(s string, left, right int) string {
	if right <= left {
		return ""
	}
	return ansi.TruncateLeftWc(ansi.TruncateWc(s, right, ""), left, "")
}
