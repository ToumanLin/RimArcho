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
