package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// 06 §9.1: Mod 库存 from the palette: the Lineage rows, a filter line, enter shows the copies.
func TestLibraryList(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	m.update(tea.KeyMsg{Type: tea.KeyCtrlK})
	typeIn(m, "mod library")
	if r := m.pal.results[0]; r.alias != "mod library" {
		t.Fatalf("palette: first result %q", r.name)
	}
	press(m, "enter")
	if m.tab != tabLibrary || m.libr.page == nil {
		t.Fatal("the palette did not open the library")
	}
	screen := strings.Join(m.libraryLines(30), "\n")
	for _, want := range []string{"Alpha", "Beta", "Harmony", "4 mods", "workshop"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("missing %q:\n%s", want, screen)
		}
	}
	press(m, "/")
	typeIn(m, "state:unu")
	if !strings.Contains(m.libraryLines(30)[0], "/ state:unused") {
		t.Fatalf("no grey completion: %q", m.libraryLines(30)[0])
	}
	m.update(tea.KeyMsg{Type: tea.KeyTab})
	if m.libr.query != "state:unused" {
		t.Fatalf("tab takes the completion: %q", m.libr.query)
	}
	press(m, "enter")
	screen = strings.Join(m.libraryLines(30), "\n")
	if strings.Contains(screen, "Alpha") || !strings.Contains(screen, "Beta") || !strings.Contains(screen, "of 4 mods") {
		t.Fatalf("filtered:\n%s", screen)
	}
	press(m, "enter")
	if m.panel == nil || !strings.Contains(m.panel.title+"", "Beta") {
		t.Fatal("enter shows the lineage")
	}
	press(m, "esc")
	press(m, "esc") // clears the filter
	if m.libr.query != "" || m.tab != tabLibrary {
		t.Fatal("esc clears the filter first")
	}
	press(m, "esc")
	if m.tab != tabList {
		t.Fatal("esc goes back")
	}
}
