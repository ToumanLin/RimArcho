package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// shift+1-8 with a Chinese input method in Chinese mode: the pages still open
// (VM round 2026-11-10: the TUI took them for unknown keys).
func TestIMEPageKeys(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	for _, c := range []struct {
		key  string
		tab  int
		name string
	}{
		{"\uff20", tabProblems, "shift+2"},
		{"\u2026\u2026", tabLibrary, "shift+6"},
		{"\uffe5", tabList, "shift+4"},
		{"\uff01", tabHome, "shift+1"},
	} {
		press(m, c.key)
		if m.tab != c.tab {
			t.Fatalf("%s (%q): tab %d, want %d", c.name, c.key, m.tab, c.tab)
		}
	}
	if keyName(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("\uff01"), Paste: true}) == "!" {
		t.Fatal("pasted text taken for a key")
	}
}
