package tui

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

	"github.com/rimarcho/rimarcho/internal/cmds"
	"github.com/rimarcho/rimarcho/internal/testkit"
)

func init() { lipgloss.SetColorProfile(termenv.Ascii) }

// fixture: Core, Harmony, A (depends on Harmony), B, a CJK-named mod, and a ModsConfig.xml
// listing Core, A, Harmony (A before its dependency, so sort has work to do).
func fixture(t *testing.T) (*testkit.Env, *int) {
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	a := testkit.Sample("tester.a", "Alpha")
	a.Deps = []string{"brrainz.harmony"}
	e.AddWorkshop("1001", a)
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), testkit.Sample("tester.b", "Beta"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Zh"), testkit.Sample("tester.zh", "中文汉化补丁（简体）"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a", "brrainz.harmony")
	e.WriteFile(filepath.Join(e.Steam, "steam.exe"), "")
	e.AddEnv(false)
	e.MustRun("scan")
	scans := new(int)
	return e, scans
}

func start(t *testing.T, e *testkit.Env, scans *int, w, h int) *teatest.TestModel {
	opts := Options{App: e.App, Scan: func(string) (string, error) {
		*scans++
		// Stand-in for the child process: the same command, run by the test.
		out, _ := cmds.Execute(e.App, []string{"scan"})
		return out, nil
	}}
	return teatest.NewTestModel(t, New(opts), teatest.WithInitialTermSize(w, h))
}

func keys(tm *teatest.TestModel, s ...string) {
	for _, k := range s {
		switch k {
		case "enter":
			tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
		case "esc":
			tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
		case "down":
			tm.Send(tea.KeyMsg{Type: tea.KeyDown})
		case "space":
			tm.Send(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
		case "ctrl+r":
			tm.Send(tea.KeyMsg{Type: tea.KeyCtrlR})
		default:
			tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		}
	}
}

func waitFor(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte(want)) },
		teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(20*time.Millisecond))
}

func finish(t *testing.T, tm *teatest.TestModel) *model {
	t.Helper()
	keys(tm, "q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	return tm.FinalModel(t).(*model)
}

func activeNames(t *testing.T, e *testkit.Env) string {
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, en := range st.List {
		out = append(out, st.Ix.ByID[en.Instance].Name)
	}
	return strings.Join(out, ", ")
}

func TestListEnableMoveSortSaveUndo(t *testing.T) {
	e, scans := fixture(t)
	tm := start(t, e, scans, 120, 30)
	waitFor(t, tm, "active 3/")
	// Rows: 1 Core, 2 Alpha, 3 Harmony, then disabled Beta, 中文…
	keys(tm, "/", "Beta", "enter", "space")
	waitFor(t, tm, "added")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony, Beta" {
		t.Fatalf("after space: %s", got)
	}
	keys(tm, "esc", "g", "down", "J") // move Alpha down one
	waitFor(t, tm, "moved")
	if got := activeNames(t, e); got != "Core, Harmony, Alpha, Beta" {
		t.Fatalf("after J: %s", got)
	}
	keys(tm, "u")
	waitFor(t, tm, "undid")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony, Beta" {
		t.Fatalf("after undo: %s", got)
	}
	keys(tm, "S")
	waitFor(t, tm, "sort — preview")
	keys(tm, "y")
	waitFor(t, tm, "applied to the workspace")
	if got := activeNames(t, e); got != "Core, Harmony, Alpha, Beta" {
		t.Fatalf("after sort: %s", got)
	}
	before := e.ReadModsConfig()
	keys(tm, "w")
	waitFor(t, tm, "save to game — preview")
	if e.ReadModsConfig() != before {
		t.Fatal("the preview wrote the file")
	}
	keys(tm, "y")
	waitFor(t, tm, "saved v2")
	if !strings.Contains(e.ReadModsConfig(), "<li>tester.b</li>") {
		t.Fatal("save did not write the list")
	}
	keys(tm, "L")
	waitFor(t, tm, "launch: dry run (steam)")
	keys(tm, "n")
	m := finish(t, tm)
	if len(e.Launched) != 0 || m.unsaved != 0 || *scans != 0 {
		t.Fatalf("launched %v, unsaved %d, scans %d", e.Launched, m.unsaved, *scans)
	}
}

func TestRescanRunsOutOfProcessAndCommandLine(t *testing.T) {
	e, scans := fixture(t)
	tm := start(t, e, scans, 120, 30)
	waitFor(t, tm, "active 3/")
	keys(tm, "R")
	waitFor(t, tm, "scan:2")
	keys(tm, ":", "scan", "enter") // the command line also routes scans to the child
	waitFor(t, tm, "scan:3")
	keys(tm, ":", "list --active", "enter")
	waitFor(t, tm, "archo list --active")
	keys(tm, "esc", "3")
	waitFor(t, tm, "changes: 0 (scan:3)")
	m := finish(t, tm)
	if *scans != 2 {
		t.Fatalf("scans via the child-process hook: %d", *scans)
	}
	_ = m
}

func TestSmallTerminalAndCJK(t *testing.T) {
	e, scans := fixture(t)
	tm := start(t, e, scans, 80, 24)
	waitFor(t, tm, "active 3/")
	keys(tm, "G") // to the CJK row at the bottom
	time.Sleep(300 * time.Millisecond)
	m := finish(t, tm)
	view := m.listLines(m.listHeight())
	for _, l := range view {
		if strings.Contains(l, "│") {
			t.Fatalf("detail pane shown at 80 columns: %q", l)
		}
		if w := lipgloss.Width(l); w > 80 {
			t.Fatalf("line wider than the terminal (%d): %q", w, l)
		}
	}
	widths := map[int]bool{}
	for _, l := range view[1:] {
		widths[lipgloss.Width(l)] = true
	}
	if len(widths) != 1 {
		t.Fatalf("rows have different display widths: %v", widths)
	}
	found := false
	for _, l := range view {
		found = found || strings.Contains(l, "中文汉化")
	}
	if !found {
		t.Fatal("CJK row not rendered")
	}
}

func TestEnvSwitchAndInspect(t *testing.T) {
	e, scans := fixture(t)
	other := filepath.Join(e.Root, "Second", "RimWorld")
	e.CopyTemplate("core", filepath.Join(other, "Data", "Core"))
	e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
	e.MustRun("env", "add", "second", "--game", other)
	tm := start(t, e, scans, 120, 30)
	waitFor(t, tm, "env: main")
	keys(tm, "g", "enter")
	waitFor(t, tm, "package: ludeon.rimworld")
	keys(tm, "esc", "E", "down", "enter")
	waitFor(t, tm, "env: second")
	m := finish(t, tm)
	if m.envName != "second" {
		t.Fatalf("env %q", m.envName)
	}
}

// keep io imported for teatest's reader type in older toolchains
var _ io.Reader

func TestChineseUI(t *testing.T) {
	e, scans := fixture(t)
	e.MustRun("config", "set", "ui.lang", "zh")
	t.Setenv("ARCHO_LANG", "")
	SetLang(e.App.Store.Meta("cfg:ui.lang"))
	t.Cleanup(func() { lang = "en" })
	tm := start(t, e, scans, 100, 24)
	waitFor(t, tm, "启用 3/")
	keys(tm, "?")
	waitFor(t, tm, "列表视图")
	keys(tm, "esc")
	m := finish(t, tm)
	for _, l := range m.listLines(m.listHeight()) {
		if w := lipgloss.Width(l); w > 100 {
			t.Fatalf("line wider than the terminal (%d): %q", w, l)
		}
	}
	// Command output stays English (05 §1).
	if out, _ := cmds.Execute(e.App, []string{"status"}); !strings.Contains(out, "lineages:") {
		t.Fatalf("status output changed: %s", out)
	}
}
