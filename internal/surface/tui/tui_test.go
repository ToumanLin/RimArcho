package tui

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func init() { lipgloss.SetColorProfile(termenv.Ascii) }

// fixture: Core, Harmony, A (depends on Harmony), B, a CJK-named mod, and a ModsConfig.xml
// listing Core, A, Harmony (A before its dependency, so sort has work to do).
func fixture(t *testing.T) (*testkit.Env, *int) {
	e := fixtureFiles(t)
	e.MustRun("scan")
	scans := new(int)
	return e, scans
}

// fixtureFiles is fixture without its scan, for fixtures that add more mods and scan once.
func fixtureFiles(t *testing.T) *testkit.Env {
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	a := testkit.Sample("tester.a", "Alpha")
	a.Deps = []string{"brrainz.harmony"}
	e.AddWorkshop("1001", a)
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), testkit.Sample("tester.b", "Beta"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Zh"), testkit.Sample("tester.zh", "中文汉化补丁（简体）加长的名字用来测试截断"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a", "brrainz.harmony")
	e.WriteFile(filepath.Join(e.Steam, "steam.exe"), "")
	e.AddEnv(false)
	return e
}

func start(t *testing.T, e *testkit.Env, scans *int, w, h int) *teatest.TestModel {
	// The stand-ins below run on the TUI's goroutines and can outlive the test's last key;
	// wait for them before the temp dir is removed (cleanups run last-registered first).
	var running sync.WaitGroup
	t.Cleanup(running.Wait)
	opts := Options{App: e.App, Scan: func(string) (string, error) {
		running.Add(1)
		defer running.Done()
		*scans++
		// Stand-in for the child process: the same command, run by the test.
		out, _ := cmds.Execute(e.App, []string{"scan"})
		return out, nil
	}, Analyze: func(env string, progress func(string)) error {
		running.Add(1)
		defer running.Done()
		// Stand-in for the `archo analyze` child: its own connection to the same database.
		a2, err := app.Open(e.Home)
		if err != nil {
			return err
		}
		defer a2.Close()
		a2.EnvName, a2.Now = env, e.App.Now
		_, err = a2.Analyze(progress)
		return err
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
		case "up":
			tm.Send(tea.KeyMsg{Type: tea.KeyUp})
		case "left":
			tm.Send(tea.KeyMsg{Type: tea.KeyLeft})
		case "right":
			tm.Send(tea.KeyMsg{Type: tea.KeyRight})
		case "tab":
			tm.Send(tea.KeyMsg{Type: tea.KeyTab})
		case "space":
			tm.Send(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
		case "ctrl+r":
			tm.Send(tea.KeyMsg{Type: tea.KeyCtrlR})
		default:
			tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		}
	}
}

// waitLimit bounds the screen waits. They return as soon as the screen matches, so the limit only
// matters when the machine is busy running many tests in parallel.
const waitLimit = 20 * time.Second

func waitFor(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte(want)) },
		teatest.WithDuration(waitLimit), teatest.WithCheckInterval(20*time.Millisecond))
}

// waitAll waits for one frame that shows every string.
func waitAll(t *testing.T, tm *teatest.TestModel, want ...string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		for _, w := range want {
			if !bytes.Contains(b, []byte(w)) {
				return false
			}
		}
		return true
	}, teatest.WithDuration(waitLimit), teatest.WithCheckInterval(20*time.Millisecond))
}

// waitAnalyzed waits for one stretch of output that shows every string and a Can it run
// heading drawn after the background analysis finished: its last heading neither says the
// files are being checked nor that they were never checked (needs.go). The heading is
// redrawn when the analysis ends, so this is the end of the analysis, not a fixed wait.
func waitAnalyzed(t *testing.T, tm *teatest.TestModel, want ...string) {
	t.Helper()
	leads := []string{"to handle before you start", "error groups in the last run", "not checked yet",
		"can run; suggestions", "can run: nothing to handle"}
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		for _, w := range want {
			if !bytes.Contains(b, []byte(w)) {
				return false
			}
		}
		at := -1
		for _, l := range leads {
			at = max(at, bytes.LastIndex(b, []byte(l)))
		}
		if at < 0 {
			return false
		}
		head := b[at:min(len(b), at+120)]
		return !bytes.Contains(head, []byte("checking the mods")) && !bytes.Contains(head, []byte("not checked yet"))
	}, teatest.WithDuration(waitLimit), teatest.WithCheckInterval(20*time.Millisecond))
}

func finish(t *testing.T, tm *teatest.TestModel) *model {
	t.Helper()
	keys(tm, "q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(waitLimit))
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

// direct builds a model without a program, for layout and timing checks.
func direct(t *testing.T, e *testkit.Env, w, h int) *model {
	m := New(Options{App: e.App, Scan: func(string) (string, error) { t.Fatal("scan"); return "", nil }}).(*model)
	m.Init()
	m.update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

// finishPage delivers a page read without running unrelated ticks or analysis commands.
func finishPage(m *model, cmd tea.Cmd) {
	if cmd != nil {
		if msg, ok := cmd().(bgMsg); ok {
			msg.apply(m)
		}
	}
}

func updatePage(m *model, msg tea.Msg) {
	seq := m.prob.seq
	_, cmd := m.update(msg)
	if m.tab == tabLibrary && m.libr.loading || m.tab == tabHome && m.home.loading || m.prob.seq != seq || m.panel != nil && m.panel.title == T("texture optimisation") {
		finishPage(m, cmd)
	}
}

func press(m *model, k string) {
	switch k {
	case "space":
		updatePage(m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	case "down":
		updatePage(m, tea.KeyMsg{Type: tea.KeyDown})
	case "up":
		updatePage(m, tea.KeyMsg{Type: tea.KeyUp})
	case "enter":
		updatePage(m, tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		updatePage(m, tea.KeyMsg{Type: tea.KeyEsc})
	case "left":
		updatePage(m, tea.KeyMsg{Type: tea.KeyLeft})
	case "right":
		updatePage(m, tea.KeyMsg{Type: tea.KeyRight})
	case "shift+up":
		updatePage(m, tea.KeyMsg{Type: tea.KeyShiftUp})
	case "shift+down":
		updatePage(m, tea.KeyMsg{Type: tea.KeyShiftDown})
	case "ctrl+up":
		updatePage(m, tea.KeyMsg{Type: tea.KeyCtrlUp})
	case "ctrl+down":
		updatePage(m, tea.KeyMsg{Type: tea.KeyCtrlDown})
	case "ctrl+space":
		updatePage(m, tea.KeyMsg{Type: tea.KeyCtrlAt})
	default:
		updatePage(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
}

func TestEnableMoveSortSaveUndo(t *testing.T) {
	t.Parallel()
	e, scans := fixture(t)
	tm := start(t, e, scans, 160, 40)
	waitFor(t, tm, "Enabled 3")
	// Enabled is focused; h goes to "Disabled", where a filter finds Beta. space enables it
	// where the order wants it: the end, nothing constrains it (16 §3.5).
	keys(tm, "h", "/", "Beta", "enter", "space")
	waitFor(t, tm, "placed at position 4, m changes it")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony, Beta" {
		t.Fatalf("after space: %s", got)
	}
	keys(tm, "esc", "l", "home", "down", "J") // to Enabled; move Alpha down one
	waitFor(t, tm, "Alpha is now number 3")
	if got := activeNames(t, e); got != "Core, Harmony, Alpha, Beta" {
		t.Fatalf("after J: %s", got)
	}
	keys(tm, "u")
	waitFor(t, tm, "undid the last change")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony, Beta" {
		t.Fatalf("after undo: %s", got)
	}
	// S, w and L act at once and say what they did (16 §6); sort offers u.
	keys(tm, "S")
	waitFor(t, tm, "sorted: 1 moved, strong requirements put right: 1, u undoes")
	if got := activeNames(t, e); got != "Core, Harmony, Alpha, Beta" {
		t.Fatalf("after sort: %s", got)
	}
	keys(tm, "w")
	waitFor(t, tm, "saved as v")
	if !strings.Contains(e.ReadModsConfig(), "<li>tester.b</li>") {
		t.Fatal("save did not write the list")
	}
	keys(tm, "L") // Start first (16 §2.5), then L there launches
	waitFor(t, tm, T("launch the game"))
	keys(tm, "L")
	waitFor(t, tm, "the game is starting")
	m := finish(t, tm)
	if len(e.Launched) != 1 || m.unsaved != 0 || *scans != 0 {
		t.Fatalf("launched %v, unsaved %d, scans %d", e.Launched, m.unsaved, *scans)
	}
}

func TestRescanRunsOutOfProcessAndCommandLine(t *testing.T) {
	t.Parallel()
	e, scans := fixture(t)
	tm := start(t, e, scans, 120, 30)
	waitFor(t, tm, "Enabled 3")
	keys(tm, "R")
	waitFor(t, tm, "scan complete")
	keys(tm, ":", "scan", "enter") // the command line also routes scans to the child
	// The second scan prints the same sentence, which the renderer does not send again: wait
	// for the child-process hook instead.
	for deadline := time.Now().Add(20 * time.Second); *scans < 2 && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	keys(tm, ":", "list --active", "enter")
	// The command help also shows its name before the asynchronous result panel opens.
	waitAll(t, tm, "archo list --active", "workspace: 3 active")
	keys(tm, "esc", "c")
	waitAll(t, tm, "updates", "the last scan", "nothing changed")
	keys(tm, "esc")
	m := finish(t, tm)
	if *scans != 2 || m.tab != tabList {
		t.Fatalf("scans via the child-process hook: %d, screen %d", *scans, m.tab)
	}
}

// 16 §11 T2 and T3: 160, 130, 100 and 78 columns degrade as 16 §2.5 says, every line fits,
// CJK names are cut on whole characters, and every list operation still works.
func TestBreakpointsAndCJK(t *testing.T) {
	e, _ := fixture(t)
	SetIcons("nerd") // the widest set: 1 cell per icon is part of the check
	t.Cleanup(func() { SetIcons("compat") })
	for _, tc := range []struct {
		w       int
		cols    string
		detail  bool
		tabs    bool
		staging bool
	}{
		{160, "0 1 2", true, false, true},
		{130, "0 1", true, false, false},
		{100, "0 1", false, false, false},
		{78, "1", false, true, false},
	} {
		t.Run(fmt.Sprint(tc.w), func(t *testing.T) {
			m := direct(t, e, tc.w, 30)
			l := m.layout()
			var kinds []string
			for _, s := range l.slots {
				kinds = append(kinds, fmt.Sprint(int(s.kind)))
			}
			if strings.Join(kinds, " ") != tc.cols || (l.detailW > 0) != tc.detail || l.tabs != tc.tabs || l.has(cStaging) != tc.staging {
				t.Fatalf("layout %+v", l)
			}
			if tc.detail && l.detailW < 40 {
				t.Fatalf("detail column %d wide", l.detailW)
			}
			check := func(what string) {
				t.Helper()
				for i, line := range strings.Split(m.View(), "\n") {
					if w := lipgloss.Width(line); w > tc.w {
						t.Fatalf("%s: line %d is %d cells: %q", what, i, w, line)
					}
					if strings.ContainsRune(line, '�') {
						t.Fatalf("%s: broken character in %q", what, line)
					}
				}
			}
			check("start")
			// Staging is reachable at every width: focus it (it takes the first slot when
			// hidden), stage a mod from "enabled", put it back.
			press(m, "right") // Enabled → Staging
			if m.focus != cStaging || !m.layout().has(cStaging) {
				t.Fatalf("staging not shown when focused: %+v", m.layout())
			}
			check("staging focused")
			press(m, "h")
			press(m, "home")
			press(m, "down") // Alpha
			press(m, "t")
			if !strings.Contains(m.status, "staged Alpha") {
				t.Fatalf("t: %q", m.status)
			}
			press(m, "right") // T in staging puts everything back (elsewhere it stages all)
			press(m, "T")
			if got := activeNames(t, e); got != "Core, Alpha, Harmony" {
				t.Fatalf("T: %s", got)
			}
			// Not enabled: the CJK row, enable and disable again.
			press(m, "h") // Staging → Enabled
			press(m, "h")
			press(m, "end")
			check("CJK row selected")
			if it := m.current(); it == nil || !strings.HasPrefix(it.name, "中文") {
				t.Fatalf("cursor on %+v", m.current())
			}
			press(m, "space") // enabled at the end, the cursor stays in Disabled
			if got := activeNames(t, e); !strings.HasSuffix(got, "中文汉化补丁（简体）加长的名字用来测试截断") || m.focus != cInactive {
				t.Fatalf("enable: %s, focus %d", got, m.focus)
			}
			for _, k := range []string{"l", "end", "space", "h"} { // Enabled, the last row, disable, back
				press(m, k)
			}
			if got := activeNames(t, e); got != "Core, Alpha, Harmony" {
				t.Fatalf("enable + disable: %s", got)
			}
			// The CJK name is cut at a character boundary: the row is exactly its column wide.
			if m.focus != cInactive {
				t.Fatalf("focus %d", m.focus)
			}
			press(m, "end")
			for _, s := range m.layout().slots {
				if s.kind != cInactive {
					continue
				}
				c := &m.cols[cInactive]
				row := m.drawRow(c, c.cursor, true, s.w)
				if w := lipgloss.Width(row); w != s.w {
					t.Fatalf("row %d cells in a %d column: %q", w, s.w, row)
				}
				if !strings.Contains(row, "中文") || (tc.w > 80 && !strings.Contains(row, "…")) {
					t.Fatalf("CJK row not cut with …: %q", row)
				}
			}
			// Details: the column, or enter for the full screen.
			if !tc.detail {
				press(m, "enter")
				if !m.full {
					t.Fatal("enter did not open the details")
				}
				check("details")
				press(m, "esc")
			}
		})
	}
}

// 16 §11 T9: 20 mods into staging (picked with v, one t), then T: all back in place.
func TestStagingT9(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	active := []string{"ludeon.rimworld"}
	for i := 1; i <= 26; i++ {
		pkg := fmt.Sprintf("tester.m%02d", i)
		e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprintf("M%02d", i)), testkit.Sample(pkg, fmt.Sprintf("M%02d", i)))
		active = append(active, pkg)
	}
	e.WriteModsConfig("1.6.4633 rev1261", active...)
	e.AddEnv(false)
	e.MustRun("scan")
	before := activeNames(t, e)
	m := direct(t, e, 160, 40)
	press(m, "home")
	press(m, "down") // M01
	for i := 0; i < 26; i++ {
		if i%4 != 3 { // picks 20 of 26; ctrl+↓ moves the cursor alone (16 §3.4)
			press(m, "v")
		}
		press(m, "ctrl+down")
	}
	if len(m.marked) != 20 {
		t.Fatalf("marked %d", len(m.marked))
	}
	press(m, "t")
	if !strings.Contains(m.status, "staged 20 mods") || len(m.cols[cStaging].items) != 20 || len(m.cols[cActive].items) != 7 {
		t.Fatalf("t: %q, staging %d, enabled %d", m.status, len(m.cols[cStaging].items), len(m.cols[cActive].items))
	}
	press(m, "l") // staging
	press(m, "T")
	if got := activeNames(t, e); got != before {
		t.Fatalf("T9:\n got %s\nwant %s", got, before)
	}
	if !strings.Contains(m.status, "put 20 mods back in place") {
		t.Fatalf("status %q", m.status)
	}
}

// 16 §3.4: a click moves the cursor (and focus), ctrl+click picks one more, shift+click
// picks a range; there is no drag-to-move; the wheel scrolls.
func TestMouseClicks(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	for i := 1; i <= 5; i++ {
		e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprintf("N%d", i)), testkit.Sample(fmt.Sprintf("tester.n%d", i), fmt.Sprintf("N%d", i)))
	}
	e.MustRun("scan")
	m := direct(t, e, 160, 30)
	m.View()
	l := m.layout()
	xOf := func(k colKind) int {
		for _, s := range l.slots {
			if s.kind == k {
				return s.x + 5
			}
		}
		t.Fatalf("no slot %d", k)
		return 0
	}
	click := func(x, y int, shift, ctrl bool) {
		m.update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Shift: shift, Ctrl: ctrl})
		m.View()
	}
	// Disabled rows start at y=4 (the filter bar is above the lists): Beta, N1 … N5, the CJK-named mod
	click(xOf(cInactive), 4, false, false)
	if m.focus != cInactive || m.current().name != "Beta" || len(m.marked) != 0 {
		t.Fatalf("click: focus %d on %+v, selected %d", m.focus, m.current(), len(m.marked))
	}
	click(xOf(cInactive), 6, false, true) // ctrl: N2
	click(xOf(cInactive), 8, false, true) // ctrl: N4
	if len(m.marked) != 2 {
		t.Fatalf("ctrl+click: %d selected", len(m.marked))
	}
	click(xOf(cInactive), 9, true, false) // shift: N4 … N5 added to the range from N4
	if names := pickedNames(m); names != "N2 N4 N5" {
		t.Fatalf("shift+click: %q", names)
	}
	click(xOf(cInactive), 4, false, false) // a plain click in the same list keeps the selection
	if names := pickedNames(m); names != "N2 N4 N5" || m.current().name != "Beta" {
		t.Fatalf("plain click: %q on %s", names, m.current().name)
	}
	// A click in another list moves the focus there and drops the selection (16 §3.4).
	click(xOf(cActive), 5, false, false)
	if len(m.marked) != 0 || m.focus != cActive || m.current().name != "Alpha" {
		t.Fatalf("click in Enabled: selected %d, %+v", len(m.marked), m.current())
	}
	if got := activeNames(t, e); got != "Core, Alpha, Harmony" {
		t.Fatalf("a click changed the list: %s", got)
	}
	// The wheel scrolls the detail wherever it is; the cursor and the focus stay (16 §3.3).
	m.update(tea.MouseMsg{X: xOf(cActive), Y: 6, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.cols[cActive].cursor != 1 || m.focus != cActive || m.detailFocus || m.detailOff != 3 {
		t.Fatalf("wheel: cursor %d, focus %d/%v, detail scrolled %d", m.cols[cActive].cursor, m.focus, m.detailFocus, m.detailOff)
	}
}

func pickedNames(m *model) string {
	var out []string
	for _, id := range m.selectedIDs() {
		out = append(out, m.st.Ix.ByID[id].Name)
	}
	return strings.Join(out, " ")
}

func TestEnvSwitchAndInspect(t *testing.T) {
	t.Parallel()
	e, scans := fixture(t)
	other := filepath.Join(e.Root, "Second", "RimWorld")
	e.CopyTemplate("core", filepath.Join(other, "Data", "Core"))
	e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
	e.MustRun("env", "add", "second", "--game", other)
	tm := start(t, e, scans, 120, 30)
	waitFor(t, tm, " main ")
	keys(tm, "home", "enter") // the full-screen detail, then I: the technical pages
	waitFor(t, tm, "ludeon.rimworld")
	keys(tm, "I")
	waitFor(t, tm, "package: ludeon.rimworld")
	keys(tm, "esc", "esc", "E", "down", "enter")
	waitFor(t, tm, "switched to second")
	m := finish(t, tm)
	if m.envName != "second" {
		t.Fatalf("env %q", m.envName)
	}
}

// 16 §2.14: E, then d on an environment: the dry run, enter removes it; the other one is
// then in use.
func TestEnvRemoveFromChooser(t *testing.T) {
	t.Parallel()
	e, scans := fixture(t)
	other := filepath.Join(e.Root, "Second", "RimWorld")
	e.CopyTemplate("core", filepath.Join(other, "Data", "Core"))
	e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
	e.MustRun("env", "add", "second", "--game", other)
	tm := start(t, e, scans, 120, 30)
	waitFor(t, tm, " main ")
	keys(tm, "E", "d")
	waitFor(t, tm, "then second is in use")
	keys(tm, "enter")
	waitFor(t, tm, "removed game main")
	m := finish(t, tm)
	if m.envName != "second" {
		t.Fatalf("env %q", m.envName)
	}
	if envs, _ := e.App.Store.Envs(); len(envs) != 1 {
		t.Fatalf("%d environments", len(envs))
	}
}

// keep io imported for teatest's reader type in older toolchains
var _ io.Reader

func TestChineseUI(t *testing.T) {
	e, scans := fixture(t)
	e.MustRun("config", "set", "ui.lang", "zh")
	t.Setenv("ARCHO_LANG", "")
	SetLang(e.App.Store.Meta("cfg:ui.lang"))
	t.Cleanup(func() { SetLang("en") })
	tm := start(t, e, scans, 100, 24)
	waitAll(t, tm, "已启用 3", "未启用 2")
	keys(tm, "?")
	waitFor(t, tm, "方向键只移动")
	keys(tm, "esc", "h", "space")
	waitFor(t, tm, "已放在第 4 位，m 改位置")
	m := finish(t, tm)
	for _, l := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(l); w > 100 {
			t.Fatalf("line wider than the terminal (%d): %q", w, l)
		}
	}
	// Command output stays English (13 §1.1).
	if out, _ := cmds.Execute(e.App, []string{"status"}); !strings.Contains(out, "lineages:") {
		t.Fatalf("status output changed: %s", out)
	}
}

// needItems: the texts of the items of Can it run.
func needItems(m *model) []string {
	var out []string
	for _, it := range m.prob.items {
		out = append(out, it.text)
	}
	return out
}

func findNeed(m *model, sub string) int {
	for i, it := range m.prob.items {
		if strings.Contains(it.text, sub) {
			return i
		}
	}
	return -1
}

// 16 §5: a cycle of order requirements is one item in "this list"; enter shows the
// requirements and the automatic resolution in words; a dependency that is installed but
// not enabled is one item in "missing dependencies" that space enables.
func TestNeedsView(t *testing.T) {
	t.Parallel()
	e, scans := fixture(t)
	for _, p := range [][2]string{{"X", "tester.y"}, {"Y", "tester.x"}} {
		m := testkit.Sample("tester."+strings.ToLower(p[0]), p[0])
		dir := filepath.Join(e.Game, "Mods", p[0])
		e.WriteMod(dir, m)
		e.WriteFile(filepath.Join(dir, "About", "About.xml"), strings.Replace(m.AboutXML(), "</ModMetaData>",
			"<loadAfter><li>"+p[1]+"</li></loadAfter></ModMetaData>", 1))
	}
	e.MustRun("scan")
	e.MustRun("enable", "pkg:tester.x", "pkg:tester.y")
	e.MustRun("disable", "pkg:brrainz.harmony")
	tm := start(t, e, scans, 140, 40)
	waitFor(t, tm, "Enabled")
	keys(tm, "@")
	waitAnalyzed(t, tm, "sorting keeps the strongest constraints")
	fm := finish(t, tm)
	if fm.prob.never || fm.prob.stale {
		t.Fatal("analysis did not complete")
	}
	ci, di := findNeed(fm, "sorting keeps the strongest constraints in a cycle of 2 mods"), findNeed(fm, "Alpha needs Harmony, but Harmony is not enabled")
	if ci < 0 || di < 0 {
		t.Fatalf("items: %q", needItems(fm))
	}
	if fm.prob.items[ci].counted {
		t.Fatal("cycle note must not count as a problem")
	}
	if fm.needCount() != countedNeeds(fm) || fm.needCount() == 0 {
		t.Fatalf("top bar %d, counted items %d", fm.needCount(), countedNeeds(fm))
	}
	// Second session: view the automatic resolution; x must not create an ignore rule.
	tm = start(t, e, scans, 140, 40)
	waitFor(t, tm, "Enabled")
	keys(tm, "@")
	waitFor(t, tm, "sorting keeps the strongest constraints")
	for i := 0; i < ci; i++ {
		keys(tm, "down")
	}
	keys(tm, "enter")
	waitFor(t, tm, "set aside by sorting")
	keys(tm, "esc", "x")
	keys(tm, "g") // keys are handled in order before finish's q; x and g run no background work
	fm2 := finish(t, tm)
	if findNeed(fm2, "sorting keeps the strongest constraints") < 0 {
		t.Fatalf("automatic cycle note absent: %q", needItems(fm2))
	}
	if out := e.MustRun("rule", "list"); strings.Contains(out, "ignore") {
		t.Fatalf("automatic resolution wrote an ignore rule: %s", out)
	}
	tm = start(t, e, scans, 140, 40)
	waitFor(t, tm, "Enabled")
	keys(tm, "@")
	waitFor(t, tm, "is not enabled")
	fm3 := finish(t, tm)
	m := direct(t, e, 140, 40)
	m.tab = tabProblems
	m.loadProblems()
	m.prob.cursor = findNeed(m, "Alpha needs Harmony")
	if m.prob.cursor < 0 {
		t.Fatalf("items: %q", needItems(fm3))
	}
	m.needsKey(" ")
	if !strings.Contains(activeNames(t, e), "Harmony") {
		t.Fatal("space did not enable Harmony")
	}
	if findNeed(m, "Alpha needs Harmony") >= 0 {
		t.Fatal("item still listed after enabling")
	}
}

func countedNeeds(m *model) int {
	n := 0
	for _, it := range m.prob.items {
		if it.counted {
			n++
		}
	}
	return n
}

// 16 §2.9: changes are a sentence on the status line after a scan and the c view, in
// words; an outside change of ModsConfig.xml is an item of Can it run with load / ignore.
func TestChangesView(t *testing.T) {
	t.Parallel()
	e, scans := fixture(t)
	e.WriteFile(filepath.Join(e.Game, "Mods", "B", "Defs", "ThingDefs", "beta.xml"), "<Defs><ThingDef><defName>beta_Changed</defName></ThingDef></Defs>\n")
	e.WriteFile(filepath.Join(e.Game, "Mods", "B", "Patches", "new.xml"), "<Patch/>\n")
	e.Advance(time.Hour)
	tm := start(t, e, scans, 120, 30)
	waitFor(t, tm, "Enabled")
	keys(tm, "R")
	waitFor(t, tm, "1 mods updated")
	keys(tm, "c")
	waitFor(t, tm, "a "+T("every mod")) // Beta is not in the plan
	keys(tm, "a")
	waitAll(t, tm, "worth a look", "Beta", "Defs, Patches changed")
	keys(tm, "enter")
	waitFor(t, tm, "Defs 1 changed; Patches 1 new")
	keys(tm, "i")
	waitAll(t, tm, "~ Defs/ThingDefs/beta.xml", "+ Patches/new.xml")
	keys(tm, "esc")
	finish(t, tm)

	e.MustRun("enable", "pkg:tester.b")                                         // an unsaved edit …
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony") // … and an outside change
	m := direct(t, e, 120, 30)
	m.tab = tabProblems
	m.loadProblems()
	i := findNeed(m, "changed outside RimArcho")
	if i < 0 {
		t.Fatalf("items: %q", needItems(m))
	}
	m.prob.cursor = i
	m.needsKey("I")
	st, _ := e.App.Loadout()
	if st.WS.Conflict != 0 || !strings.Contains(m.status, "ignored") {
		t.Fatalf("I did not dismiss the conflict: %q", m.status)
	}
	m.loadProblems()
	if findNeed(m, "changed outside") >= 0 {
		t.Fatal("still listed")
	}
}

// 16 §2.4, §7.3: the current log's errors are one item per mod under "last run"; a stale
// log is one dim row that is not counted.
func TestNeedsLogs(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	scans := new(int)
	tm := start(t, e, scans, 160, 50)
	keys(tm, "@")
	waitAnalyzed(t, tm, "last run")
	fm := finish(t, tm)
	last := 0
	for _, it := range fm.prob.items {
		if it.group == view.GroupLastRun {
			last++
			if !it.counted || !strings.Contains(it.text, " — ") || len(it.keys) == 0 || it.keys[0].label != "open game log" {
				t.Fatalf("log item %+v", it)
			}
		}
	}
	if last == 0 {
		t.Fatalf("no log items: %q", needItems(fm))
	}

	e.Advance(time.Hour)
	e.MustRun("disable", "pkg:tester.manual")
	m := direct(t, e, 160, 50)
	m.tab = tabProblems
	m.app.LazyLog, m.app.LogCacheOnly = false, false
	m.loadProblems()
	stale := 0
	for _, it := range m.prob.items {
		if it.group == view.GroupLastRun {
			if it.counted || !strings.HasPrefix(it.text, "earlier log") {
				t.Fatalf("stale log item %+v", it)
			}
			stale++
		}
	}
	if stale != 1 {
		t.Fatalf("stale rows %d: %q", stale, needItems(m))
	}
}

// 16 §7: every icon of both sets is one cell wide for RimArcho's width function.
func TestIconWidths(t *testing.T) {
	t.Parallel()
	for name, set := range map[string]icons{"nerd": nerdIcons, "compat": compatIcons} {
		for i, s := range []string{set.official, set.workshop, set.local, set.localCopy, set.git, set.err, set.warn, set.logErr,
			set.unloadable, set.hint, set.unsaved, set.lock, set.update, set.dep, set.staging, set.play, set.save, set.sort, set.search, set.fail,
			set.evDeclared, set.evInferred, set.evLog, set.evCommunity, set.evRule,
			markSel, markOK, markPrompt, markBusy} {
			if s == "" {
				continue // "not shown" in compatibility mode
			}
			if w := width.StringWidth(s); w != 1 {
				t.Errorf("%s icon %d %q is %d cells", name, i, s, w)
			}
		}
	}
}

func TestWrap(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		w    int
		want string
	}{
		{"staging is empty, t puts the selected mod here", 20, "staging is empty, t|puts the selected|mod here"},
		{"暂存是空的，按 t 把选中的 Mod 放进来", 12, "暂存是空的，|按 t 把选中|的 Mod 放进|来"},                                           // CJK breaks between characters
		{"使用Harmony库的模组", 10, "使用|Harmony库|的模组"},                                                              // never inside a Latin word
		{`  path: D:\SteamLibrary\steamapps\workshop`, 20, `  path:|  D:\SteamLibrary\|  steamapps\workshop`}, // indent kept; paths break after \
		{"中文中文中文", 5, "中文|中文|中文"},
	} {
		got := strings.Join(wrap(tc.in, tc.w), "|")
		if got != tc.want {
			t.Errorf("wrap(%q, %d) = %q, want %q", tc.in, tc.w, got, tc.want)
		}
		for _, l := range wrap(tc.in, tc.w) {
			if width.StringWidth(l) > tc.w {
				t.Errorf("line %q wider than %d", l, tc.w)
			}
		}
	}
}

// 16 §3.7 on the fixture: every field, negation, AND, the per-column query, the fallback.
func TestColumnSearch(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	e.MustRun("tag", "remove", "pkg:ludeon.rimworld", "RimWorld") // the default tag (16 §3.8): not under test here
	e.MustRun("tag", "add", "pkg:brrainz.harmony", "core-lib", "--mod", "pkg:tester.b")
	e.MustRun("note", "pkg:tester.b", "check later")
	e.WriteFile(filepath.Join(e.Game, "Mods", "B", "Assemblies", "Beta.dll"), "MZ")
	e.WriteFile(filepath.Join(e.Game, "Mods", "Zh", "Languages", "ChineseSimplified", "Keyed", "a.xml"), "<LanguageData/>")
	e.Remove(filepath.Join(e.Game, "Mods", "Zh", "Defs")) // a language pack: only Languages/
	e.Remove(filepath.Join(e.Game, "Mods", "Zh", "Patches"))
	e.MustRun("scan")
	m := direct(t, e, 160, 30)
	names := func(k colKind) string {
		var out []string
		for _, i := range m.cols[k].view {
			out = append(out, m.cols[k].items[i].name)
		}
		return strings.Join(out, ",")
	}
	all := func(q string) string {
		m.bar.search = q // the filter bar filters Not enabled and Enabled together (16 §2.5)
		m.barChanged()
		return names(cActive) + " | " + names(cInactive)
	}
	for q, want := range map[string]string{
		"":                         "Core,Alpha,Harmony | Beta,中文汉化补丁（简体）加长的名字用来测试截断",
		"alp":                      "Alpha | ",
		"tag:core-lib":             "Harmony | Beta",
		"-tag:core-lib":            "Core,Alpha | 中文汉化补丁（简体）加长的名字用来测试截断",
		"tag:core-lib type:local":  " | Beta",
		"type:workshop":            "Alpha,Harmony | ",
		"type:official":            "Core | ",
		"id:tester.":               "Alpha | Beta,中文汉化补丁（简体）加长的名字用来测试截断",
		"id:1001":                  "Alpha | ",
		"author:nobody":            " | ",
		"has:tag":                  "Harmony | Beta",
		"has:note":                 " | Beta",
		"type:assembly":            " | Beta",
		"type:language":            " | 中文汉化补丁（简体）加长的名字用来测试截断",
		"type:language-pack":       " | 中文汉化补丁（简体）加长的名字用来测试截断",
		"type:xml-only -id:ludeon": "Alpha,Harmony | 中文汉化补丁（简体）加长的名字用来测试截断",
		"-type:assembly type:xml":  "Core,Alpha,Harmony | 中文汉化补丁（简体）加长的名字用来测试截断",
		"中文":                       " | 中文汉化补丁（简体）加长的名字用来测试截断",
		"hmy":                      "Harmony | ", // the fallback: letters in order
		`"alpha"`:                  "Alpha | ",
	} {
		m.content = view.ContentCache{} // each query loads what it needs itself
		if got := all(q); got != want {
			t.Errorf("%q: got %q, want %q", q, got, want)
		}
	}
	// id:#ref
	ref := m.cols[cActive].items[1].ref
	if got := all("id:" + ref); got != "Alpha | " {
		t.Errorf("id:%s: %q", ref, got)
	}
	// The bar's query holds for both lists; the headers say N / M; esc clears it.
	all("")
	m.focus = cInactive
	press(m, "/")
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tag:core-lib")})
	press(m, "enter")
	press(m, "l")
	if names(cActive) != "Harmony" || names(cInactive) != "Beta" {
		t.Fatalf("both lists: %q / %q", names(cActive), names(cInactive))
	}
	title := m.columnTitle(&m.cols[cInactive], false, 40)
	if !strings.Contains(title, "Disabled 1 / 2") {
		t.Fatalf("title %q", title)
	}
	press(m, "esc")
	if names(cInactive) == "Beta" || names(cActive) == "Harmony" {
		t.Fatal("esc did not clear the bar")
	}
}

// 16 §6: L loads an outside change of ModsConfig.xml into the list at once; it is one
// step, so u brings the player's list back.
func TestLoadExternalAtOnce(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	e.MustRun("enable", "pkg:tester.b")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	m := direct(t, e, 120, 30)
	m.tab = tabProblems
	m.loadProblems()
	m.prob.cursor = findNeed(m, "changed outside RimArcho")
	before := activeNames(t, e)
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	st, _ := e.App.Loadout()
	if st.WS.Conflict != 0 || activeNames(t, e) != "Core, Harmony" || m.armed != nil || !strings.Contains(m.status, T("u undoes")) {
		t.Fatalf("after L: conflict %d, list %q, status %q", st.WS.Conflict, activeNames(t, e), m.status)
	}
	if _, _, err := e.App.UndoRedo(false); err != nil || activeNames(t, e) != before {
		t.Fatalf("undo: %v, list %q, want %q", err, activeNames(t, e), before)
	}
}
