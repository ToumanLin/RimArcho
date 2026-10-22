package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// 06 §3.5: one key table for the editor and Mod 库存; ? lists the page's keys; shift+1…7
// (the characters of the US layout) go to pages; tab / shift+tab go to the next page under
// the same sidebar entry.
func TestKeyTable(t *testing.T) {
	e, _ := fixture(t)
	e.MustRun("plan", "new", "second")
	e.MustRun("plan", "use", "default")
	m := direct(t, e, 160, 40)
	help := func() string {
		press(m, "?")
		if m.panel == nil {
			t.Fatal("no help")
		}
		s := strings.Join(m.panel.lines, "\n")
		m.panel = nil
		return s
	}
	if h := help(); !strings.Contains(h, `\`) || !strings.Contains(h, "J K") || strings.Contains(h, "a mod's copies") {
		t.Fatalf("editor help:\n%s", h)
	}
	// g: tags; ctrl+a: every row shown; n: the mod's note
	press(m, "2")
	press(m, "g")
	if m.tags == nil {
		t.Fatal("g did not open the tag bar")
	}
	m.tags = nil
	m.update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if m.selCount() != 3 {
		t.Fatalf("ctrl+a: %d", m.selCount())
	}
	press(m, "esc")
	press(m, "n")
	if m.prompt == nil {
		t.Fatal("n: no prompt")
	}
	typeIn(m, "keep it")
	press(m, "enter")
	if st, _ := e.App.Loadout(); st.Ix.ByID[m.current().id].Note != "keep it" {
		t.Fatalf("note: %q", m.status)
	}
	// tab: the next plan
	press(m, "tab")
	if m.st.Plan.Name != "second" {
		t.Fatalf("tab: plan %s", m.st.Plan.Name)
	}
	press(m, "shift+tab")
	if m.st.Plan.Name != "default" {
		t.Fatalf("shift+tab: plan %s", m.st.Plan.Name)
	}
	// shift+2 能不能跑, shift+4 Mod 库存, tab 变化, tab back, shift+5 方案
	press(m, "@")
	if m.tab != tabProblems {
		t.Fatalf("@: tab %d", m.tab)
	}
	press(m, "$")
	if m.tab != tabLibrary {
		t.Fatalf("$: tab %d", m.tab)
	}
	if h := help(); !strings.Contains(h, "a mod's copies") || strings.Contains(h, "J K") {
		t.Fatalf("library help:\n%s", h)
	}
	press(m, "tab")
	if m.tab != tabChanges {
		t.Fatalf("tab in the library: %d", m.tab)
	}
	press(m, "tab")
	if m.tab != tabLibrary {
		t.Fatalf("tab in changes: %d", m.tab)
	}
	press(m, "%")
	if m.tab != tabList {
		t.Fatalf("%%: tab %d", m.tab)
	}
	press(m, "!")
	if !strings.Contains(m.status, "window") {
		t.Fatalf("!: %q", m.status)
	}
	// enter opens and closes the full detail
	press(m, "enter")
	if !m.full {
		t.Fatal("enter: no full detail")
	}
	press(m, "enter")
	if m.full {
		t.Fatal("enter again: still full")
	}
}
