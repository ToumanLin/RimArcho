package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"

	tea "github.com/charmbracelet/bubbletea"
)

// 16 §2.14, §9.1: uninstall RimArcho in the palette shows the plan; enter asks for the word; any
// other word cancels, "uninstall" runs it, and after it every key quits.
func TestUninstallPalette(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	e.WriteFile(filepath.Join(e.Root, "LocalAppData", "RimArcho", "steamcmd", deps.SteamCMDExecutable()), "MZ")
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
	_, next := m.update(enter)
	for m.uninstallRun != nil {
		if next == nil {
			t.Fatal("uninstall stopped receiving events")
		}
		_, next = m.update(next())
	}
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

func TestUninstallIgnoresQuitKeysWhileWorkerRuns(t *testing.T) {
	t.Parallel()
	for _, commandLine := range []bool{false, true} {
		name := "panel"
		if commandLine {
			name = "command-line"
		}
		t.Run(name, func(t *testing.T) { testUninstallQuitKeys(t, commandLine) })
	}
}

func testUninstallQuitKeys(t *testing.T, commandLine bool) {
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	reached, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	e.App.AfterExitFunc = func(app.AfterExit) error { close(reached); <-release; return nil }
	var next tea.Cmd
	if commandLine {
		typeText(m, ":")
		typeText(m, "uninstall --yes --env=main")
		next = keyCmd(m, tea.KeyEnter)
	} else {
		next = m.startUninstall(app.UninstallOptions{})
	}
	run := m.uninstallRun
	if run == nil || next == nil {
		t.Fatal("execution did not enter guarded worker")
	}
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not run")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyCtrlC}, {Type: tea.KeyRunes, Runes: []rune("q")}, {Type: tea.KeyEsc}} {
		_, cmd := m.update(key)
		if cmd != nil || m.quitting || m.uninstallRun != run || m.panel == nil {
			t.Fatalf("key %s interrupted uninstall", key.String())
		}
		if m.status != T("uninstalling; you can close RimArcho when it finishes") {
			t.Fatalf("missing notice: %s", m.status)
		}
	}
	before := m.tab
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	if m.cmdOn || m.tab != before {
		t.Fatal("navigation remained usable")
	}
	unblock()
	for m.uninstallRun != nil {
		if next == nil {
			t.Fatal("missing next event")
		}
		_, next = m.update(next())
	}
	if !m.app.Uninstalled || m.panel == nil {
		t.Fatal("report missing")
	}
	if _, cmd := m.update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil || !m.quitting {
		t.Fatal("quit not restored")
	}
}

// More progress than the queue can hold must not prevent completion if the
// terminal stops consuming input/output halfway through the uninstall.
func TestUninstallWorkerFinishesWithoutTerminal(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	for i := 0; i < 20; i++ {
		e.WriteFile(filepath.Join(e.Home, "packs", fmt.Sprintf("synthetic-%d", i), "pack.json"), `{ "format":1,"entries":[] }`)
	}
	e.App.AfterExitFunc = func(app.AfterExit) error { return nil }
	m.startUninstall(app.UninstallOptions{})
	run := m.uninstallRun
	select {
	case <-run.done:
	case <-time.After(10 * time.Second):
		t.Fatal("uninstall worker blocked on a missing terminal")
	}
	found := false
	for len(run.ch) > 0 {
		if _, ok := (<-run.ch).(uninstallDoneMsg); ok {
			found = true
		}
	}
	if !found {
		t.Fatal("completion lost behind progress")
	}
}

func TestUninstallChangedAnswersRequireReview(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	m.uninstallPanel()
	m.uninstallPrompt()
	typeIn(m, "uninstall --remove-packs")
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.uninstallRun != nil || m.app.Uninstalled || m.panel == nil || m.panel.onEnter == nil {
		t.Fatal("changed options executed before review")
	}
	text := strings.Join(m.panel.lines, "\n")
	if !strings.Contains(text, T("will remove or change")) || !strings.Contains(text, T("will keep")) || !strings.Contains(text, T("answers changed; review the updated summary before typing uninstall")) {
		t.Fatalf("matching confirmation summary missing: %s", text)
	}
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	typeIn(m, "cancel")
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.uninstallRun != nil || m.app.Uninstalled {
		t.Fatal("cancel executed uninstall")
	}
}

func TestUninstallReviewsQuotedDestination(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	destination := filepath.Join(e.Root, "synthetic kept packs")
	e.WriteFile(filepath.Join(e.Home, "packs", "synthetic", "pack.json"), `{ "format":1,"entries":[] }`)
	m.uninstallPanel()
	m.uninstallPrompt()
	typeIn(m, `uninstall --packs-to "`+destination+`"`)
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.uninstallRun != nil || m.panel == nil || m.panel.onEnter == nil || !strings.Contains(strings.Join(m.panel.lines, "\n"), destination) {
		t.Fatal("quoted destination was not reviewed before execution")
	}
	e.App.AfterExitFunc = func(app.AfterExit) error { return nil }
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	typeIn(m, "uninstall")
	_, next := m.update(tea.KeyMsg{Type: tea.KeyEnter})
	for m.uninstallRun != nil {
		if next == nil {
			t.Fatal("missing completion event")
		}
		_, next = m.update(next())
	}
	if !m.app.Uninstalled {
		t.Fatalf("confirmed choices did not execute: %s", m.status)
	}
	if _, err := os.Stat(filepath.Join(destination, "synthetic", "pack.json")); err != nil {
		t.Fatalf("did not use reviewed destination: %v", err)
	}
}

// Results of other work that arrive while an uninstall runs are delivered once it fails, so
// the scan and the command line do not stay busy.
func TestUninstallFailureDeliversHeldResults(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	m.scanning = true
	m.tasks.scanning.Store(true)
	m.openCmdLine() // the command line keeps its history
	press(m, "esc")
	m.cmdBusy = "check"
	m.uninstallRun = &uninstallRun{ch: make(chan tea.Msg, 1), done: make(chan struct{})}
	m.update(scanDoneMsg{})
	m.update(cmdDoneMsg{line: "check", argv: []string{"check"}})
	if !m.scanning || m.cmdBusy == "" {
		t.Fatal("results applied during the run")
	}
	_, cmd := m.update(uninstallDoneMsg{err: fmt.Errorf("synthetic failure")})
	var deliver func(tea.Cmd)
	deliver = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				deliver(c)
			}
		case scanDoneMsg, cmdDoneMsg:
			m.update(msg)
		}
	}
	deliver(cmd)
	if m.scanning || m.tasks.scanning.Load() || m.cmdBusy != "" {
		t.Fatalf("still busy after a failed uninstall: scan %v, command %q", m.scanning, m.cmdBusy)
	}
}
