package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Selection (06 §13.5): the selected instances of the focused list, m.marked. It is a
// selection only, not a mode: plain ↑↓ move the cursor and keep it, shift+↑↓ extend it as
// a range from where the range started, ctrl+space adds or removes the cursor's row, esc
// clears it, and going to another list clears it. Verbs act on it (or on the cursor's row
// when nothing is selected).

// selRange: where a shift range started, and the rows selected before it began.
type selRange struct {
	anchor string
	base   map[string]bool
}

func (m *model) selCount() int { return len(m.marked) }

// clearSel drops the selection.
func (m *model) clearSel() {
	m.marked, m.sel = map[string]bool{}, selRange{}
}

// selToggle adds or removes the cursor's row (ctrl+space, ctrl+click, v).
func (m *model) selToggle() tea.Cmd {
	id := m.col().curID()
	if id == "" {
		return nil
	}
	if m.marked[id] {
		delete(m.marked, id)
	} else {
		m.marked[id] = true
	}
	m.sel = selRange{}
	return nil
}

// selExtend moves the cursor by d and selects every row from where the range started.
func (m *model) selExtend(d int) tea.Cmd {
	c := m.col()
	if c.curID() == "" {
		return nil
	}
	if m.sel.anchor == "" {
		m.sel.anchor, m.sel.base = c.curID(), map[string]bool{}
		for id := range m.marked {
			m.sel.base[id] = true
		}
	}
	cmd := m.moveCursor(d)
	m.selRangeTo()
	return cmd
}

// selRangeTo: the rows selected before the range plus every row from the anchor to the cursor.
func (m *model) selRangeTo() {
	c := m.col()
	a, b := -1, c.cursor
	for i, vi := range c.view {
		if c.items[vi].id == m.sel.anchor {
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
	for id := range m.sel.base {
		m.marked[id] = true
	}
	for i := a; i <= b && i < len(c.view); i++ {
		if id := c.items[c.view[i]].id; id != "" {
			m.marked[id] = true
		}
	}
}

// selectedIDs: the selected instances in the order of the focused list.
func (m *model) selectedIDs() []string {
	var out []string
	for _, it := range m.cols[m.focus].items {
		if it.id != "" && m.marked[it.id] {
			out = append(out, it.id)
		}
	}
	return out
}

// focusOn puts the cursor on an instance in whichever list holds it.
func (m *model) focusOn(id string) {
	for k := range m.cols {
		for _, it := range m.cols[k].items {
			if it.id == id {
				m.setFocus(colKind(k))
				m.refilterCol(colKind(k), id)
				if m.cols[k].curID() != id { // hidden by the list's search
					m.cols[k].filter = ""
					m.refilterCol(colKind(k), id)
				}
				m.cols[k].scroll(m.rowsShown() - 1)
				return
			}
		}
	}
}
