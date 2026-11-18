package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// On Can it run the selected item's keys come before the global ones: its R (an enabled Mod
// replaces it, repair the links, unhide) is not a rescan (VM 2026-11-10).
func TestNeedItemKeyBeforeGlobal(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 140, 40) // its Scan fails the test
	m.tab = tabProblems
	m.loadProblems()
	ran := ""
	m.prob.items = append([]need{{text: "item", keys: []needKey{
		{key: "R", run: func(*model) tea.Cmd { ran += "R"; return nil }},
		{key: "E", run: func(*model) tea.Cmd { ran += "E"; return nil }},
	}}}, m.prob.items...)
	m.prob.cursor = 0
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	if ran != "RE" {
		t.Fatalf("item keys ran %q", ran)
	}
	// q still quits: the item's keys never take it.
	m.prob.items[0].keys = append(m.prob.items[0].keys, needKey{key: "q", run: func(*model) tea.Cmd { t.Fatal("q went to the item"); return nil }})
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !m.quitting {
		t.Fatal("q did not quit")
	}
}

// The item's keys are asked once, in key(), before the global R (rescan) and the page keys;
// needsKey itself only has the page's keys. direct's Scan fails the test if R rescans.
func TestNeedItemKeyRunsOnce(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	m.tab = tabProblems
	m.loadProblems()
	runs := 0
	m.prob.items = append([]need{{text: "item", keys: []needKey{
		{key: "R", run: func(*model) tea.Cmd { runs++; return nil }},
	}}}, m.prob.items...)
	m.prob.cursor = 0
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if runs != 1 {
		t.Fatalf("R on the selected item ran %d times", runs)
	}
	if m.needsKey("R"); runs != 1 {
		t.Fatal("needsKey ran the item's key")
	}
	// another item selected: R is the item's no longer
	m.prob.items[0].keys = nil
	if _, ok := m.needItemKey("R"); ok {
		t.Fatal("an item without R took it")
	}
}

// pressNeed presses s on Can it run the way the player does, through key(): the selected
// item's keys, then the global keys, then the page's.
func pressNeed(m *model, s string) {
	switch s {
	case "enter":
		m.key(tea.KeyMsg{Type: tea.KeyEnter})
	case " ", "space":
		m.key(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	default:
		m.key(runes(s))
	}
}
