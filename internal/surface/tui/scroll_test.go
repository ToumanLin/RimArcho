package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// T15 (06 §12.6): in the full-screen detail, ↓ to the bottom then → shows the next mod
// from the top; tab / shift+tab go through the sub-sections; the wheel scrolls; esc goes
// back with the cursor on the last mod shown.
func TestT15FullScreenNext(t *testing.T) {
	e, _ := fixture(t)
	e.MustRun("rule", "add", "after", "pkg:tester.a", "pkg:brrainz.harmony")
	m := direct(t, e, 120, 16)
	press(m, "g")
	press(m, "down") // Alpha
	_, c := m.update(tea.KeyMsg{Type: tea.KeyEnter})
	runCmd(m, c) // the sub-sections load
	m.View()
	for i := 0; i < 200; i++ {
		press(m, "down")
	}
	m.View()
	if m.fullOff == 0 {
		t.Fatal("the full-screen detail did not scroll")
	}
	_, c = m.update(tea.KeyMsg{Type: tea.KeyRight})
	if m.current().name != "Harmony" || m.fullOff != 0 {
		t.Fatalf("→: %s at %d", m.current().name, m.fullOff)
	}
	runCmd(m, c)
	out := ansi.Strip(m.View())
	if !strings.Contains(strings.Join(strings.Split(out, "\n")[:4], "\n"), "Harmony") {
		t.Fatalf("not from the top:\n%s", out)
	}
	// tab: the sub-sections, then back to the summary
	starts := m.fullStarts()
	if len(starts) < 3 {
		t.Fatalf("sub-sections %v", starts)
	}
	press(m, "tab")
	if m.fullSec != 1 || m.fullOff != starts[1] {
		t.Fatalf("tab: section %d at %d, want 1 at %d", m.fullSec, m.fullOff, starts[1])
	}
	press(m, "tab")
	press(m, "shift+tab")
	press(m, "shift+tab")
	if m.fullSec != 0 || m.fullOff != 0 {
		t.Fatalf("shift+tab: section %d at %d", m.fullSec, m.fullOff)
	}
	m.update(tea.MouseMsg{X: 10, Y: 8, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.fullOff != 3 {
		t.Fatalf("wheel: %d", m.fullOff)
	}
	press(m, "left")
	press(m, "right")
	press(m, "esc")
	if m.full || m.focus != cActive || m.current().name != "Harmony" {
		t.Fatalf("esc: full %v, %s", m.full, m.current().name)
	}
}

// 06 §12.5: → on Enabled gives the detail the focus; ↑↓ scroll it, the name stays on top
// in the accent colour; ← goes back; a click on the detail gives it the focus too.
func TestDetailFocus(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 160, 10)
	press(m, "g")
	press(m, "down") // Alpha
	runCmd(m, m.loadData(m.current().id, false, false))
	press(m, "right")
	if !m.detailFocus || m.focus != cActive {
		t.Fatalf("→: detail %v, focus %d", m.detailFocus, m.focus)
	}
	press(m, "down")
	press(m, "down")
	out := ansi.Strip(m.View())
	if m.detailOff != 2 || !strings.Contains(out, "▲ Alpha") {
		t.Fatalf("scroll %d:\n%s", m.detailOff, out)
	}
	if m.current().name != "Alpha" {
		t.Fatalf("↓ moved the list cursor to %s", m.current().name)
	}
	press(m, "left")
	if m.detailFocus || m.focus != cActive {
		t.Fatalf("←: detail %v focus %d", m.detailFocus, m.focus)
	}
	if strings.Contains(ansi.Strip(m.View()), "▲ Alpha") {
		t.Fatal("still scrolled after ←")
	}
	l := m.layout()
	m.update(tea.MouseMsg{X: l.detailX + 5, Y: 6, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.detailFocus {
		t.Fatal("a click did not focus the detail")
	}
	press(m, "esc")
	press(m, "h") // Enabled → Disabled: an ordinary column move again
	if m.focus != cInactive || m.detailFocus {
		t.Fatalf("h: focus %d detail %v", m.focus, m.detailFocus)
	}
	press(m, "l") // Disabled → Enabled (→ enters the detail from Enabled and Staging only)
	if m.focus != cActive || m.detailFocus {
		t.Fatalf("l from Disabled: focus %d detail %v", m.focus, m.detailFocus)
	}
}
