package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// 16 §3.7: from the second letter the search shows the rest of a field name; Tab takes it.
func TestSearchCompletes(t *testing.T) {
	t.Parallel()
	e, _ := v3Fixture(t)
	m := direct(t, e, 160, 40)
	press(m, "1")
	press(m, "/")
	typeIn(m, "au")
	if g := m.bar.ghost; g != "thor:" {
		t.Fatalf("ghost %q", g)
	}
	pressKey(m, tea.KeyTab)
	if f := m.bar.search; f != "author:" {
		t.Fatalf("filter %q after Tab", f)
	}
	typeIn(m, "x ty")
	pressKey(m, tea.KeyTab)
	if f := m.bar.search; f != "author:x type:" {
		t.Fatalf("filter %q after second Tab", f)
	}
}
