package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The history area (08 §14, 10 §6): where the staging list is when staging is empty, the
// unsaved steps around "now" (redo dimmed above, undo below), then the saved versions. The
// rows are view.HistoryItems; enter on a step goes there at once (u / ctrl+r move back), on
// a version asks first (press twice).

// loadHistory reads the history area when staging is empty; the cursor starts on "now".
func (m *model) loadHistory() {
	m.hist, m.histCur, m.histOff = nil, 0, 0
	if m.st == nil || len(m.cols[cStaging].items) > 0 {
		return
	}
	h, err := view.HistoryItems(m.app, m.st)
	if err != nil {
		return
	}
	m.hist = h
	for i, it := range h {
		if it.Kind == "now" {
			m.histCur = i
		}
	}
}

// historyShown: staging is empty, not searched, and there is more than "now" to show.
func (m *model) historyShown() bool {
	c := &m.cols[cStaging]
	return len(c.items) == 0 && c.filter == "" && len(m.hist) > 1
}

// histActive: the history area has the keys (staging focused and showing it).
func (m *model) histActive() bool {
	return m.focus == cStaging && !m.detailFocus && m.historyShown()
}

// histLines draws the history area in at most n lines of w cells, keeping the cursor's row
// in view.
func (m *model) histLines(w, n int) []string {
	if n <= 1 {
		return nil
	}
	head := func(t string) string { return "  " + sDimS.Render(fit(T(t), w-2)) }
	rows := []string{head("history")}
	at := []int{-1}
	versions := false
	for i, h := range m.hist {
		if h.Kind == "version" && !versions {
			versions = true
			rows, at = append(rows, head("saved versions")), append(at, -1)
		}
		st := sText
		switch h.Kind {
		case "redo":
			st = sDimS
		case "now":
			st = sAccent
		}
		lead := "  "
		if i == m.histCur && m.histActive() {
			lead, st = sAccent.Render(markSel)+" ", sSel
		}
		rows, at = append(rows, lead+st.Render(fit(h.Text, w-2))), append(at, i)
	}
	cur := 0
	for i, a := range at {
		if a == m.histCur {
			cur = i
		}
	}
	if cur < m.histOff {
		m.histOff = max(cur-1, 0)
	}
	if cur >= m.histOff+n {
		m.histOff = cur - n + 1
	}
	m.histOff = max(0, min(m.histOff, max(len(rows)-n, 0)))
	end := min(m.histOff+n, len(rows))
	return rows[m.histOff:end]
}

// histKey: the keys of the history area; ok false leaves the key to the list.
func (m *model) histKey(s string) (tea.Cmd, bool) {
	switch s {
	case "up", "k":
		m.histCur = max(m.histCur-1, 0)
	case "down", "j":
		m.histCur = min(m.histCur+1, len(m.hist)-1)
	case "home", "g":
		m.histCur = 0
	case "end", "G":
		m.histCur = len(m.hist) - 1
	case "enter":
		return m.histGo(), true
	default:
		return nil, false
	}
	return nil, true
}

// histLabel is what enter does on the cursor's row ("" on "now").
func (m *model) histLabel() string {
	if m.histCur >= len(m.hist) {
		return ""
	}
	switch m.hist[m.histCur].Kind {
	case "now":
		return ""
	case "version":
		return T("load this version into the list")
	}
	return T("go back to here")
}

// histGo goes to the cursor's row: a step at once, a version after a second enter.
func (m *model) histGo() tea.Cmd {
	if m.histCur >= len(m.hist) {
		return nil
	}
	h := m.hist[m.histCur]
	switch h.Kind {
	case "now":
		return nil
	case "version":
		warn := T("load this version into the list") + ": " + h.Text + " " + markDot + " " + T("your current list stays in the history; u brings it back")
		return m.arm("enter", warn, func(m *model) tea.Cmd {
			_, _, err := m.app.Checkout(fmt.Sprintf("v%d", h.Version))
			m.reload()
			if err != nil {
				m.say(sayFail, m.humanize(err.Error()))
			} else {
				m.say(sayOK, Tf("loaded v%d into the list (not saved yet)", h.Version))
			}
			return m.requestDetail()
		})
	}
	r, err := m.app.StepTo(h.Steps)
	m.reload()
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	m.say(sayOK, Tf("%d changes", len(r.Changes)))
	return m.requestDetail()
}
