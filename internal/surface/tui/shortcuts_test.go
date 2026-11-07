package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// cursorOn puts the cursor on the mod named name, in whichever list has it.
func cursorOn(t *testing.T, m *model, name string) {
	t.Helper()
	for k := range m.cols {
		c := &m.cols[k]
		for v, i := range c.view {
			if c.items[i].name == name {
				m.goArea(k + 1)
				c.cursor = v
				return
			}
		}
	}
	t.Fatalf("no %s in the lists", name)
}

// 16 §3.5: ctrl+o opens the cursor's mod's folder, ctrl+w its Workshop page (the status
// line says when it has no Workshop ID), C the Updates page narrowed to its Lineage; the
// same in Mod library.
func TestShortcutsFolderWorkshopChanges(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	var dir, url string
	m.opts.OpenDir = func(d string) error { dir = d; return nil }
	m.opts.OpenURL = func(u string) error { url = u; return nil }

	cursorOn(t, m, "Alpha")
	m.update(tea.KeyMsg{Type: tea.KeyCtrlO})
	if filepath.Base(dir) != "1001" {
		t.Fatalf("ctrl+o opened %q", dir)
	}
	m.update(tea.KeyMsg{Type: tea.KeyCtrlW})
	if url != "https://steamcommunity.com/sharedfiles/filedetails/?id=1001" {
		t.Fatalf("ctrl+w opened %q", url)
	}
	m.update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if url != "steam://url/CommunityFilePage/1001" {
		t.Fatalf("ctrl+s opened %q", url)
	}
	url = ""
	m.runEditorOp("steam")
	if url != "steam://url/CommunityFilePage/1001" {
		t.Fatalf("Steam menu opened %q", url)
	}

	cursorOn(t, m, "Beta")
	url = ""
	m.update(tea.KeyMsg{Type: tea.KeyCtrlW})
	if url != "" || !strings.Contains(m.status, "no Workshop ID") {
		t.Fatalf("ctrl+w on a local mod: opened %q, said %q", url, m.status)
	}
	m.update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if url != "" || !strings.Contains(m.status, "no Workshop ID") {
		t.Fatalf("ctrl+s on a local mod: opened %q, said %q", url, m.status)
	}

	// C: only Beta's changes; esc first shows every mod, then leaves
	press(m, "C")
	if m.tab != tabChanges || m.chg.of == "" {
		t.Fatalf("C: tab %v of %q", m.tab, m.chg.of)
	}
	beta := 0
	for _, r := range m.chg.rows {
		if r.mod != nil && !strings.Contains(r.text, "Beta") {
			t.Fatalf("another mod's change: %q", r.text)
		}
		if r.kind == chDay {
			beta++
		}
	}
	if beta == 0 {
		t.Fatalf("no day listed for Beta: %+v", m.chg.rows)
	}
	if !strings.Contains(m.chg.rows[m.chg.off+1].text, "Beta") {
		t.Fatalf("history link did not land on Beta's history: %+v", m.chg)
	}
	press(m, "esc")
	if m.tab != tabChanges || m.chg.of != "" {
		t.Fatalf("esc: tab %v of %q", m.tab, m.chg.of)
	}
	if strings.Contains(m.chg.rows[0].text, "Beta") {
		t.Fatalf("still only Beta: %q", m.chg.rows[0].text)
	}
	press(m, "esc")
	if m.tab != tabList {
		t.Fatal("esc leaves Updates")
	}

	// Mod library: the cursor's Lineage
	m.openLibrary()
	for i, r := range m.libr.page.Rows {
		if r.Name == "Alpha" {
			m.libr.cursor = i
		}
	}
	dir, url = "", ""
	m.update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m.update(tea.KeyMsg{Type: tea.KeyCtrlW})
	if filepath.Base(dir) != "1001" || !strings.HasSuffix(url, "?id=1001") {
		t.Fatalf("library: folder %q page %q", dir, url)
	}
	m.update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if url != "steam://url/CommunityFilePage/1001" {
		t.Fatalf("library ctrl+s opened %q", url)
	}
}
