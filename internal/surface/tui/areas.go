package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Focus areas and the position line (06 §13.3): 1 Disabled, 2 Enabled, 3 Staging,
// 4 the detail. Number keys jump to an area, ←→ go to the neighbouring one. The arrows only
// ever navigate (06 §13.2 rule 1).

// area is the focused area, 1–4.
func (m *model) area() int {
	if m.detailFocus {
		return 4
	}
	return int(m.focus) + 1
}

// hasDetailArea: the detail is a column on this screen (not below 110 cells).
func (m *model) hasDetailArea() bool { return m.layout().detailW > 0 }

// setFocus focuses a list; another list than before drops the selection (06 §13.5).
func (m *model) setFocus(k colKind) {
	if k != m.focus {
		m.clearSel()
		m.detailRef = ""
	}
	m.focus, m.detailFocus = k, false
}

// goArea focuses area a (1–4); nothing happens for an area this screen does not have.
func (m *model) goArea(a int) tea.Cmd {
	switch {
	case a >= 1 && a <= 3:
		m.setFocus(colKind(a - 1))
		m.col().scroll(m.rowsShown() - 1)
		return m.requestDetail()
	case a == 4:
		if !m.hasDetailArea() {
			m.say(sayInfo, T("the terminal is too narrow for the detail column · enter shows the details"))
			return nil
		}
		m.detailFocus = true
	}
	return nil
}

// areaStep: ←→ move to the neighbouring area, without wrapping. ← from the detail goes
// back to the list whose mod it shows (06 §13.3, a ruling in decisions-tuiv2 §V3).
func (m *model) areaStep(d int) tea.Cmd {
	if m.detailFocus {
		if d < 0 {
			m.detailFocus = false
		}
		return nil
	}
	a := m.area() + d
	if a < 1 || a > 4 || (a == 4 && !m.hasDetailArea()) {
		return nil
	}
	return m.goArea(a)
}

// areaCycle: tab / shift+tab go round all the areas this screen has.
func (m *model) areaCycle(d int) tea.Cmd {
	n := 3
	if m.hasDetailArea() {
		n = 4
	}
	a := m.area()
	if m.detailFocus && d > 0 {
		a = n
	}
	return m.goArea((a-1+d+n)%n + 1)
}

// scrollDetail: PgUp PgDn and the wheel scroll the detail from any area, without moving
// the focus (06 §13.4). With no detail column they page the list instead.
func (m *model) scrollDetail(d int) tea.Cmd {
	if !m.hasDetailArea() {
		return m.moveCursor(d)
	}
	m.syncDetailOff()
	m.detailOff = max(m.detailOff+d, 0)
	return nil
}

// syncDetailOff: the detail of another mod starts from the top.
func (m *model) syncDetailOff() {
	if id := m.col().curID(); id != m.detailOffID {
		if m.detailOffID != "" {
			m.detailOff = 0
		}
		m.detailOffID = id
	}
}

// detailPage is how far PgUp PgDn scroll the detail column.
func (m *model) detailPage() int { return max(m.rowsShown()-2, 1) }

// ---- the position line ----

// position says where the player is (06 §13.3): the focused area and the place in it, what
// is selected, and the popup that is open.
func (m *model) position() string {
	var p string
	c := m.col()
	idx := func() string { return fmt.Sprintf("%d / %d", min(c.cursor+1, len(c.view)), len(c.view)) }
	name := func() string {
		if it := m.current(); it != nil {
			return it.name
		}
		return "—"
	}
	switch {
	case m.tab == tabProblems:
		p = T("needs attention")
		if n := len(m.prob.items); n > 0 {
			p += " " + markDot + " " + fmt.Sprintf("%d / %d", min(m.prob.cursor+1, n), n)
		}
	case m.tab == tabChanges:
		p = T("recent changes")
	case m.full || m.insp != nil:
		p = m.focus.title() + " › " + name() + Tf(" (%s)", idx())
		if m.insp != nil {
			p += " › " + T("technical pages")
		} else if s := m.fullSecName(m.fullSec); s != "" {
			p += " › " + s
		}
	case m.detailFocus:
		p = T("details") + " " + markDot + " " + name()
		if s := m.detailSection(); s != "" {
			p += " " + markDot + " " + s
		}
	default:
		p = m.focus.title() + " " + markDot + " " + idx()
		if c.filter != "" {
			p += " " + markDot + " " + T("filtered")
		}
	}
	if n := m.selCount(); n > 0 && m.tab == tabList {
		p += " " + markDot + " " + Tf("selected %d", n)
	}
	if pop := m.popupName(); pop != "" {
		p += " › " + pop
	}
	return p
}

// popupName is the open popup (06 §13.2 rule 5) or input, "" when there is none.
func (m *model) popupName() string {
	switch {
	case m.pal != nil:
		return T("command palette")
	case m.tags != nil:
		return T("tags")
	case m.pos != nil:
		return T("move to…")
	case m.prompt != nil:
		return strings.TrimRight(strings.TrimSpace(m.prompt.label), ":：")
	case m.panel != nil:
		return m.panel.title
	case m.editing:
		return T("search")
	case m.armed != nil:
		return T("confirm")
	}
	return ""
}

// detailSection: the detail section at the top of the scrolled detail and how far down it
// is, "description 40%".
func (m *model) detailSection() string {
	n := m.rowsShown() - 1
	lines := m.lastLines
	maxOff := len(lines) - n
	if maxOff <= 0 {
		return ""
	}
	sec := T("summary")
	heads := []string{T("description"), T("history"), T("technical information")}
	for i := 0; i <= m.detailOff && i < len(lines); i++ {
		t := strings.TrimSpace(ansi.Strip(lines[i].text))
		for _, h := range heads {
			if strings.HasPrefix(t, h) {
				sec = h
			}
		}
	}
	return fmt.Sprintf("%s %d%%", sec, 100*min(m.detailOff, maxOff)/maxOff)
}
