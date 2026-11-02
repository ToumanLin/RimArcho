package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The history area (06 §2.5): where the staging list is when staging is empty, the
// unsaved steps around "now" (redo dimmed above, undo below), then the saved versions. The
// rows are view.HistoryItems; enter on a step or version goes there at once
// (u / ctrl+r move back).

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
		return T("go back to this version")
	}
	return T("go back to here")
}

// chooseHistory is H: every step and version as a chooser, the window's history picker
// (06 §2.5); enter goes there, as enter in the history area does. It works whatever the
// staging area holds.
func (m *model) chooseHistory() tea.Cmd {
	if m.st == nil {
		return nil
	}
	items, err := view.HistoryItems(m.app, m.st)
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	if len(items) < 2 {
		m.say(sayInfo, T("no history yet"))
		return nil
	}
	p := &panel{title: T("history")}
	for i, it := range items {
		label := it.Text
		switch it.Kind {
		case "version":
			label = T("saved versions") + "  " + label
		case "now":
			p.choice = i
		}
		p.choices = append(p.choices, label)
	}
	p.onPickAt = func(m *model, i int) tea.Cmd { return m.histGoTo(items[i]) }
	m.panel = p
	return nil
}

// histGo goes to the cursor's row at once: a step, or a version loaded into the list.
func (m *model) histGo() tea.Cmd {
	if m.histCur >= len(m.hist) {
		return nil
	}
	return m.histGoTo(m.hist[m.histCur])
}

func (m *model) histGoTo(h view.HistItem) tea.Cmd {
	switch h.Kind {
	case "now":
		return nil
	case "version":
		// One step: the list it replaces stays in the history, u brings it back (06 §6).
		r, _, err := m.app.Checkout(fmt.Sprintf("v%d", h.Version))
		m.reload()
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
		} else if r.Before.Equal(r.State.List) {
			m.say(sayOK, T("no changes"))
		} else {
			m.say(sayOK, Tf("back at v%d (unsaved)", h.Version)+Tf(", %s", T("u undoes")))
		}
		return m.requestDetail()
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
