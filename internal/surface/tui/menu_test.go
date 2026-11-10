package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// 16 §8.5: the row menu, from the menu key, shift+F10 or a right click: the op table's
// groups in order with separators, keys on the right, only what applies, unsubscribe last; ↑↓ and
// enter run an item.
func TestRowMenu(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	press(m, "2")
	press(m, "down") // Alpha, from the Workshop
	m.update(tea.KeyMsg{Type: tea.KeyF10})
	if m.menu == nil {
		t.Fatal("F10: no menu")
	}
	var codes []string
	seps := 0
	for _, it := range m.menu.items {
		if it.sep {
			seps++
			continue
		}
		codes = append(codes, it.code)
	}
	if codes[0] != view.OpDisable || codes[len(codes)-1] != view.OpUnsub || seps < 3 {
		t.Fatalf("items %v (%d separators)", codes, seps)
	}
	screen := ansi.Strip(m.View())
	if !strings.Contains(screen, "space") || !strings.Contains(screen, "ctrl+o") || !strings.Contains(screen, "Del") {
		t.Fatalf("menu not drawn with its keys:\n%s", screen)
	}
	press(m, "esc")
	if m.menu != nil {
		t.Fatal("esc did not close it")
	}
	// shift+F10, then enter on the first item: disable
	m.update(tea.KeyMsg{Type: tea.KeyF10, Alt: false})
	press(m, "enter")
	if got := activeNames(t, e); got != "Core, Harmony" {
		t.Fatalf("menu › disable: %s (%q)", got, m.status)
	}
	// a right click on a row opens its menu there; a click on an item runs it
	press(m, "1")
	m.View()
	l := m.layout()
	x := l.slots[0].x + 5
	m.update(tea.MouseMsg{X: x, Y: 5, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
	if m.menu == nil || m.menu.items[0].code != view.OpEnable {
		t.Fatalf("right click: %+v", m.menu)
	}
	// down to stage (enable, enable at…, stage), enter
	press(m, "down")
	press(m, "down")
	press(m, "enter")
	if !strings.Contains(m.status, "staged") {
		t.Fatalf("menu › stage: %q", m.status)
	}
}
