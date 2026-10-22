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
	typeIn(m, "used:no")
	if !strings.Contains(m.libraryLines(30)[0], "/ used:none") {
		t.Fatalf("no grey completion: %q", m.libraryLines(30)[0])
	}
	m.update(tea.KeyMsg{Type: tea.KeyTab})
	if m.libr.query != "used:none" {
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

// 06 §2.9: n writes a copy's own note; an empty line clears it.
func TestLibraryCopyNote(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	m.openLibrary()
	at := -1
	for i, r := range m.libr.page.Rows {
		if r.Name == "Beta" {
			at = i
		}
	}
	if at < 0 {
		t.Fatal("no Beta row")
	}
	m.libr.cursor = at
	press(m, "n")
	if m.prompt == nil {
		t.Fatal("n opens no input line")
	}
	m.prompt.text = ""
	typeIn(m, "kept for the test plan")
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.libr.page.Rows[at].Insts[0].Note; got != "kept for the test plan" {
		t.Fatalf("note %q (status %q)", got, m.status)
	}
	press(m, "n")
	m.prompt.text = ""
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.libr.page.Rows[at].Insts[0].Note; got != "" {
		t.Fatalf("not cleared: %q", got)
	}
}
