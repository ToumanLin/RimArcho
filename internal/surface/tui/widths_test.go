package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func slotW(t *testing.T, m *model, k colKind) int {
	t.Helper()
	for _, s := range m.layout().slots {
		if s.kind == k {
			return s.w
		}
	}
	t.Fatalf("column %d not shown", k)
	return 0
}

// T16 (06 §12.6): column widths and name wrapping survive a restart; the detail column
// keeps at least 40 cells; = resets the widths.
func TestT16WidthsPersist(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 160, 30)
	w0 := slotW(t, m, cActive)
	for i := 0; i < 3; i++ {
		press(m, ">")
	}
	if got := slotW(t, m, cActive); got != w0+6 {
		t.Fatalf("> ×3: %d, want %d", got, w0+6)
	}
	press(m, "h")
	press(m, "z") // wrap names in Disabled
	m.View()
	lines := m.rowMap[cInactive]
	zh := -1
	for y, vi := range lines {
		if vi >= 0 && strings.HasPrefix(m.cols[cInactive].items[m.cols[cInactive].view[vi]].name, "中文") {
			if zh >= 0 {
				zh = -2 // two lines: wrapped
				break
			}
			zh = y
		}
	}
	if zh != -2 {
		t.Fatalf("the long name did not wrap: %v", lines)
	}

	m2 := direct(t, e, 160, 30) // a restart
	if got := slotW(t, m2, cActive); got != w0+6 {
		t.Fatalf("after restart: %d, want %d", got, w0+6)
	}
	if !m2.prefs.Wrap[cInactive] || m2.prefs.Wrap[cActive] {
		t.Fatalf("wrap after restart: %v", m2.prefs.Wrap)
	}
	// Far too wide: the others shrink and the detail keeps 40 cells.
	for i := 0; i < 60; i++ {
		press(m2, ">")
	}
	if l := m2.layout(); l.detailW < minDetailW || l.detailX+l.detailW != 160 {
		t.Fatalf("detail %d at %d", l.detailW, l.detailX)
	}
	for _, line := range strings.Split(m2.View(), "\n") {
		if w := ansi.StringWidthWc(line); w > 160 {
			t.Fatalf("a line is %d cells wide", w)
		}
	}
	press(m2, "=")
	if got := slotW(t, m2, cActive); got != w0 {
		t.Fatalf("= : %d, want %d", got, w0)
	}
}

// Dragging the gap right of a column sets its width (06 §12.1).
func TestResizeDrag(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 160, 30)
	m.View()
	l := m.layout()
	s := l.slots[0]
	gx := s.x + s.w + 1
	m.update(tea.MouseMsg{X: gx, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.update(tea.MouseMsg{X: gx + 7, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m.update(tea.MouseMsg{X: gx + 7, Y: 5, Action: tea.MouseActionRelease})
	if got := slotW(t, m, s.kind); got != s.w+7 {
		t.Fatalf("drag: %d, want %d", got, s.w+7)
	}
	if m.focus != cActive {
		t.Fatalf("a gap press moved the focus to %d", m.focus)
	}
	if direct(t, e, 160, 30).prefs.W[s.kind] != s.w+7 {
		t.Fatal("the dragged width was not stored")
	}
}
