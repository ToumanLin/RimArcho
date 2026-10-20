package tui

import "encoding/json"

// Column widths and name wrapping (06 §12.1): < > change the focused list column by 2
// cells, = resets them, a mouse drag on the gap after a column sets its width; z wraps
// long names in the focused column. Both are kept per environment (meta "ui.columns:<id>").

const (
	minColW    = 12 // narrowest list column
	minDetailW = 40 // the detail column takes the rest, never less than this
)

// colPrefs is what is stored: 0 means the default width.
type colPrefs struct {
	W    [3]int  `json:"w"`
	Wrap [3]bool `json:"wrap"`
}

func colPrefsKey(envID string) string { return "ui.columns:" + envID }

// loadColPrefs reads the environment's column settings when the environment changes.
func (m *model) loadColPrefs(envID string) {
	if envID == m.prefsEnv {
		return
	}
	m.prefsEnv, m.prefs = envID, colPrefs{}
	if v := m.app.Store.Meta(colPrefsKey(envID)); v != "" {
		_ = json.Unmarshal([]byte(v), &m.prefs)
	}
}

func (m *model) saveColPrefs() {
	if m.prefsEnv == "" {
		return
	}
	b, _ := json.Marshal(m.prefs)
	if err := m.app.Store.SetMeta(colPrefsKey(m.prefsEnv), string(b)); err != nil {
		m.say(sayFail, err.Error())
	}
}

// fitWidths applies the stored widths to the defaults and shrinks the columns, widest
// first and the focused one last, until they fit in avail.
func (m *model) fitWidths(kinds []colKind, defaults []int, avail int) []int {
	ws := make([]int, len(kinds))
	sum := 0
	for i, k := range kinds {
		ws[i] = defaults[i]
		if w := m.prefs.W[k]; w > 0 {
			ws[i] = max(w, minColW)
		}
		sum += ws[i]
	}
	for sum > avail {
		best := -1
		for i, k := range kinds {
			if ws[i] <= minColW {
				continue
			}
			if best < 0 || (k != m.focus && (kinds[best] == m.focus || ws[i] > ws[best])) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		ws[best]--
		sum--
	}
	return ws
}

// resizeKey: < > = on the focused list column.
func (m *model) resizeKey(s string) {
	l := m.layout()
	if l.tabs {
		m.say(sayInfo, T("the terminal is too narrow for column widths"))
		return
	}
	if s == "=" {
		m.prefs.W = [3]int{}
		m.saveColPrefs()
		m.say(sayOK, T("column widths reset"))
		return
	}
	for _, sl := range l.slots {
		if sl.kind != m.focus {
			continue
		}
		d := 2
		if s == "<" {
			d = -2
		}
		m.setColW(sl.kind, sl.w+d)
		m.saveColPrefs()
	}
}

// colWant: the stored width of a column, or its default.
func (m *model) colWant(k colKind, def int) int {
	if w := m.prefs.W[k]; w > 0 {
		return w
	}
	return def
}

func (m *model) setColW(k colKind, w int) {
	m.prefs.W[k] = max(w, minColW)
}

// wrapKey: z turns name wrapping on or off in the focused column.
func (m *model) wrapKey() {
	m.prefs.Wrap[m.focus] = !m.prefs.Wrap[m.focus]
	m.saveColPrefs()
	if m.prefs.Wrap[m.focus] {
		m.say(sayOK, Tf("%s: long names wrap", m.focus.title()))
	} else {
		m.say(sayOK, Tf("%s: long names are cut", m.focus.title()))
	}
}

// gapAt: the list column whose right-hand gap is at x (a resize handle).
func (m *model) gapAt(x int) (colKind, int, bool) {
	l := m.layout()
	if l.tabs {
		return 0, 0, false
	}
	for i, s := range l.slots {
		end := m.w
		switch {
		case i+1 < len(l.slots):
			end = l.slots[i+1].x
		case l.detailW > 0:
			end = l.detailX
		default:
			continue // the last column with no detail: nothing to its right
		}
		if x >= s.x+s.w && x < end {
			return s.kind, s.w, true
		}
	}
	return 0, 0, false
}

// rowHeights: how many lines each row of a wrapped column takes.
func (m *model) rowLines(c *column, vi, w int) int {
	if !m.prefs.Wrap[c.kind] {
		return 1
	}
	_, nameW := rowPrefix(c, w)
	it := &c.items[c.view[vi]]
	name := it.name
	if it.missing {
		name += " " + T("(not installed)")
	}
	if width.StringWidth(name) <= nameW {
		return 1
	}
	return max(len(wrap(name, nameW)), 1)
}

// rowPrefix: the cells before the name, and the cells left for it.
func rowPrefix(c *column, w int) (int, int) {
	used := 2 + 3 // mark, icon
	if c.kind == cActive {
		used += 6
	}
	return used, max(w-used-2, 1)
}
