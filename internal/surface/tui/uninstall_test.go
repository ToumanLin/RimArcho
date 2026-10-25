package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// 06 §2.14, §9.1: 卸载 RimArcho in the palette shows the plan; enter asks for the word; any
// other word cancels, "uninstall" runs it, and after it every key quits.
func TestUninstallPalette(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	e.WriteFile(filepath.Join(e.Root, "LocalAppData", "RimArcho", "steamcmd", "steamcmd.exe"), "MZ")
	m := direct(t, e, 140, 40)
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	m.update(tea.KeyMsg{Type: tea.KeyCtrlK})
	typeIn(m, "uninstall")
	if r := m.pal.results[0]; r.alias != "uninstall RimArcho" {
		t.Fatalf("first result %q", r.name)
	}
	m.update(enter)
	if m.panel == nil || !strings.Contains(strings.Join(m.panel.lines, "\n"), "steamcmd") || m.panel.onEnter == nil {
		t.Fatalf("no plan panel: %+v", m.panel)
	}
	m.update(enter)
	if m.prompt == nil {
		t.Fatal("enter does not ask for the word")
	}
	typeIn(m, "yes")
	m.update(enter)
	if m.app.Uninstalled || !strings.Contains(m.status, "cancelled") {
		t.Fatalf("another word must cancel: %q", m.status)
	}
	if _, err := os.Stat(filepath.Join(e.Root, "LocalAppData", "RimArcho", "steamcmd")); err != nil {
		t.Fatal("cancelled, but SteamCMD went")
	}

	m.panel = nil
	m.uninstallPrompt()
	typeIn(m, "uninstall")
	m.update(enter)
	if !m.app.Uninstalled || m.panel == nil {
		t.Fatalf("typed uninstall did not run it: %q", m.status)
	}
	if _, err := os.Stat(filepath.Join(e.Root, "LocalAppData", "RimArcho", "steamcmd")); err == nil {
		t.Fatal("SteamCMD is left")
	}
	m.update(tea.KeyMsg{Type: tea.KeyEsc})
	if _, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}); cmd == nil || !m.quitting {
		t.Fatal("a key after the report must quit")
	}
}
