package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func libAt(t *testing.T, m *model, name string) {
	t.Helper()
	l := &m.libr
	for i, x := range l.flat {
		if l.page.Rows[x.row].Name == name {
			l.cursor = i
			return
		}
	}
	t.Fatalf("no row %q", name)
}

// 06 §2.9, §9.1: Mod 库存 is the window's page in text: the filter bar (the editor's),
// the Lineage rows, the cursor's copy on the right; esc backs out a level at a time.
func TestLibraryList(t *testing.T) {
	t.Parallel()
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
	screen := ansi.Strip(strings.Join(m.libraryLines(30), "\n"))
	for _, want := range []string{"Alpha", "Beta", "Harmony", "4 mods", "workshop", "details", "F10 menu"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("missing %q:\n%s", want, screen)
		}
	}
	press(m, "/")
	typeIn(m, "used:no")
	if !strings.Contains(ansi.Strip(m.libraryLines(30)[0]), "used:none") {
		t.Fatalf("no grey completion: %q", ansi.Strip(m.libraryLines(30)[0]))
	}
	m.update(tea.KeyMsg{Type: tea.KeyTab})
	if m.libr.bar.search != "used:none" {
		t.Fatalf("tab takes the completion: %q", m.libr.bar.search)
	}
	press(m, "enter")
	screen = ansi.Strip(strings.Join(m.libraryLines(30), "\n"))
	if strings.Contains(screen, "Alpha") || !strings.Contains(screen, "Beta") || !strings.Contains(screen, "of 4 mods") {
		t.Fatalf("filtered:\n%s", screen)
	}
	// the bar's groups, as in the editor: a source left out with x
	press(m, `\`)
	if !m.libr.bar.open || len(m.libr.bar.groups) == 0 {
		t.Fatal("no groups")
	}
	press(m, "esc")
	// enter: the detail full screen, enter again back
	press(m, "enter")
	if !m.libr.full || !strings.Contains(ansi.Strip(strings.Join(m.libraryLines(30), "\n")), "Beta") {
		t.Fatal("enter: no full detail")
	}
	press(m, "enter")
	if m.libr.full {
		t.Fatal("enter again: still full")
	}
	press(m, "esc") // clears the search
	if m.libr.bar.search != "" || m.tab != tabLibrary {
		t.Fatal("esc clears the filter first")
	}
	press(m, "esc")
	if m.tab != tabList {
		t.Fatal("esc goes back")
	}
}

// 06 §2.9: n writes a copy's own note; an empty line clears it.
func TestLibraryCopyNote(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	m.openLibrary()
	libAt(t, m, "Beta")
	press(m, "n")
	if m.prompt == nil {
		t.Fatal("n opens no input line")
	}
	m.prompt.text = ""
	typeIn(m, "kept for the test plan")
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if i, _ := m.cursorCopy(); i == nil || i.Note != "kept for the test plan" {
		t.Fatalf("note %+v (status %q)", i, m.status)
	}
	press(m, "n")
	m.prompt.text = ""
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if i, _ := m.cursorCopy(); i.Note != "" {
		t.Fatalf("not cleared: %q", i.Note)
	}
}

// 06 §3.5: space adds and enables a copy, or takes it out of the current plan; u undoes;
// t stages; m enables at a place; F10 lists the copy's ops.
func TestLibraryPlanOps(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	m.openLibrary()
	libAt(t, m, "Beta")
	press(m, "space")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony, Beta" {
		t.Fatalf("space: %s (%q)", got, m.status)
	}
	libAt(t, m, "Beta")
	press(m, "space")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony" {
		t.Fatalf("space again: %s (%q)", got, m.status)
	}
	press(m, "u")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony, Beta" {
		t.Fatalf("u: %s", got)
	}
	libAt(t, m, "Beta")
	press(m, "t")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony" || !strings.Contains(m.status, "staged") {
		t.Fatalf("t: %s %q", got, m.status)
	}
	libAt(t, m, "Beta")
	press(m, "space") // put back from staging
	libAt(t, m, "Beta")
	press(m, "m")
	if m.pos == nil {
		t.Fatal("m: no position input")
	}
	typeIn(m, "1")
	press(m, "enter")
	if got := activeNames(t, e); got != "Beta, Core, Alpha, Harmony" {
		t.Fatalf("m 1: %s (%q)", got, m.status)
	}
	libAt(t, m, "Alpha")
	press(m, "f10")
	if m.menu == nil {
		t.Fatal("F10: no menu")
	}
	var codes []string
	for _, it := range m.menu.items {
		if !it.sep {
			codes = append(codes, it.code)
		}
	}
	if codes[0] != view.OpTakeOut || codes[len(codes)-1] != view.OpUnsub {
		t.Fatalf("menu %v", codes)
	}
	press(m, "esc")
	// Del: the dry run, then enter does it (here: Steam is out of reach, the page opens);
	// an enabled copy only warns which list has it
	press(m, "delete")
	if m.panel == nil || m.panel.onEnter == nil || !strings.Contains(strings.Join(m.panel.lines, "\n"), "is in list") {
		t.Fatalf("Del on an enabled copy: %+v", m.panel)
	}
	press(m, "esc")
	var opened string
	m.opts.OpenURL = func(u string) error { opened = u; return nil }
	press(m, "delete")
	if m.panel == nil || m.panel.onEnter == nil {
		t.Fatalf("Del: no confirmation (%q)", m.status)
	}
	press(m, "enter")
	if !strings.Contains(opened, "1001") {
		t.Fatalf("unsubscribe: opened %q, status %q", opened, m.status)
	}
}

// 06 §3.1 rule 6: every op of the table has a runner on both pages (no unknown code).
func TestEveryOpRuns(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	for _, o := range view.OpTable() {
		if !view.OpKeyed(o.Code) {
			t.Errorf("%s: no key and no menu", o.Code)
		}
		m.status = ""
		m.openLibrary()
		m.libr.marked = map[string]bool{}
		m.panel, m.pos, m.prompt, m.tags = nil, nil, nil, nil
		m.runLibOp(o.Code)
		if strings.Contains(m.status, "unknown") {
			t.Errorf("library: %s: %q", o.Code, m.status)
		}
		m.tab = tabList
		m.panel, m.pos, m.prompt, m.tags = nil, nil, nil, nil
		m.runEditorOp(o.Code)
		if strings.Contains(m.status, "unknown") {
			t.Errorf("editor: %s: %q", o.Code, m.status)
		}
		m.panel, m.pos, m.prompt, m.tags = nil, nil, nil, nil
	}
}
