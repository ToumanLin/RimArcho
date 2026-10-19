package tui

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

	"github.com/rimarcho/rimarcho/internal/app"
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
	e.WriteMod(filepath.Join(e.Game, "Mods", "Zh"), testkit.Sample("tester.zh", "中文汉化补丁（简体）加长的名字用来测试截断"))
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
	}, Analyze: func(env string, progress func(string)) error {
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

func waitFor(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte(want)) },
		teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(20*time.Millisecond))
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
	}, teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(20*time.Millisecond))
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

// direct builds a model without a program, for layout and timing checks.
func direct(t *testing.T, e *testkit.Env, w, h int) *model {
	m := New(Options{App: e.App, Scan: func(string) (string, error) { t.Fatal("scan"); return "", nil }}).(*model)
	m.Init()
	m.update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func press(m *model, k string) {
	switch k {
	case "space":
		m.update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	case "down":
		m.update(tea.KeyMsg{Type: tea.KeyDown})
	case "up":
		m.update(tea.KeyMsg{Type: tea.KeyUp})
	case "enter":
		m.update(tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		m.update(tea.KeyMsg{Type: tea.KeyEsc})
	default:
		m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
}

func TestEnableMoveSortSaveUndo(t *testing.T) {
	e, scans := fixture(t)
	tm := start(t, e, scans, 160, 40)
	waitFor(t, tm, "Enabled 3")
	// Enabled is focused; h goes to "Disabled", where a filter finds Beta.
	keys(tm, "h", "/", "Beta", "enter", "space")
	waitFor(t, tm, "enabled Beta")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony, Beta" {
		t.Fatalf("after space: %s", got)
	}
	keys(tm, "esc", "l", "g", "down", "J") // move Alpha down one
	waitFor(t, tm, "Alpha is now number 3")
	if got := activeNames(t, e); got != "Core, Harmony, Alpha, Beta" {
		t.Fatalf("after J: %s", got)
	}
	keys(tm, "u")
	waitFor(t, tm, "undid the last change")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony, Beta" {
		t.Fatalf("after undo: %s", got)
	}
	keys(tm, "S")
	waitFor(t, tm, "sort — preview")
	keys(tm, "y")
	waitFor(t, tm, "sorted the list")
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
	waitFor(t, tm, "saved to the game")
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
	waitFor(t, tm, "Enabled 3")
	keys(tm, "R")
	waitFor(t, tm, "scan complete")
	keys(tm, ":", "scan", "enter") // the command line also routes scans to the child
	waitFor(t, tm, "scan:3")
	keys(tm, ":", "list --active", "enter")
	waitFor(t, tm, "archo list --active")
	keys(tm, "esc", "c")
	waitAll(t, tm, "recent changes", "scan:3  2026-10-01 10:00  no changes")
	keys(tm, "esc")
	m := finish(t, tm)
	if *scans != 2 || m.tab != tabList {
		t.Fatalf("scans via the child-process hook: %d, screen %d", *scans, m.tab)
	}
}

// 06 §11 T2 and T3: 160, 130, 100 and 78 columns degrade as §5.1 says, every line fits,
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
			press(m, "l") // Enabled → Staging
			if m.focus != cStaging || !m.layout().has(cStaging) {
				t.Fatalf("staging not shown when focused: %+v", m.layout())
			}
			check("staging focused")
			press(m, "h")
			press(m, "g")
			press(m, "down") // Alpha
			press(m, "t")
			if !strings.Contains(m.status, "staged Alpha") {
				t.Fatalf("t: %q", m.status)
			}
			press(m, "T")
			if got := activeNames(t, e); got != "Core, Alpha, Harmony" {
				t.Fatalf("T: %s", got)
			}
			// Not enabled: the CJK row, enable and disable again.
			press(m, "h")
			press(m, "G")
			check("CJK row selected")
			if it := m.current(); it == nil || !strings.HasPrefix(it.name, "中文") {
				t.Fatalf("cursor on %+v", m.current())
			}
			press(m, "space")
			press(m, "l")
			press(m, "G")
			press(m, "space")
			if got := activeNames(t, e); got != "Core, Alpha, Harmony" {
				t.Fatalf("enable + disable: %s", got)
			}
			// The CJK name is cut at a character boundary: the row is exactly its column wide.
			press(m, "h")
			press(m, "G")
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

// 06 §11 T9: 20 mods into staging (marked with v, one t), then T: all back in place.
func TestStagingT9(t *testing.T) {
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
	press(m, "g")
	press(m, "down") // M01
	for i := 0; i < 26; i++ {
		if i%4 != 3 && i < 26 { // marks 20 of 26 (v moves down)
			press(m, "v")
		} else {
			press(m, "down")
		}
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

// 06 §5.3: drag between columns enables at the drop row, drags within "enabled" move,
// a click focuses and selects; the wheel scrolls.
func TestMouseDrag(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 160, 30)
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
	mouse := func(x, y int, a tea.MouseAction) {
		m.update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: a})
	}
	// Disabled row 0 is Beta (rows start at y=3); drop it on enabled row 1 (Alpha, number 2).
	mouse(xOf(cInactive), 3, tea.MouseActionPress)
	if m.focus != cInactive || m.current().name != "Beta" {
		t.Fatalf("click: focus %d on %+v", m.focus, m.current())
	}
	mouse(xOf(cActive), 4, tea.MouseActionMotion)
	if !strings.Contains(m.status, "enable Beta at number 2") {
		t.Fatalf("drag feedback %q", m.status)
	}
	mouse(xOf(cActive), 4, tea.MouseActionRelease)
	if got := activeNames(t, e); got != "Core, Beta, Alpha, Harmony" {
		t.Fatalf("drop: %s", got)
	}
	// Within enabled: Harmony (row 3) onto Beta's row (number 2).
	mouse(xOf(cActive), 6, tea.MouseActionPress)
	mouse(xOf(cActive), 4, tea.MouseActionMotion)
	mouse(xOf(cActive), 4, tea.MouseActionRelease)
	if got := activeNames(t, e); got != "Core, Harmony, Beta, Alpha" {
		t.Fatalf("reorder: %s", got)
	}
	// Enabled → staging, staging → enabled at the end.
	mouse(xOf(cActive), 5, tea.MouseActionPress) // Beta
	mouse(xOf(cStaging), 3, tea.MouseActionMotion)
	mouse(xOf(cStaging), 3, tea.MouseActionRelease)
	if got := activeNames(t, e); got != "Core, Harmony, Alpha" || len(m.cols[cStaging].items) != 1 {
		t.Fatalf("to staging: %s", got)
	}
	mouse(xOf(cStaging), 3, tea.MouseActionPress)
	mouse(xOf(cActive), 20, tea.MouseActionMotion)
	mouse(xOf(cActive), 20, tea.MouseActionRelease)
	if got := activeNames(t, e); got != "Core, Harmony, Alpha, Beta" {
		t.Fatalf("from staging: %s", got)
	}
	// A press and release on the same row is a click, not a drop.
	mouse(xOf(cActive), 3, tea.MouseActionPress)
	mouse(xOf(cActive), 3, tea.MouseActionRelease)
	if got := activeNames(t, e); got != "Core, Harmony, Alpha, Beta" {
		t.Fatalf("click moved something: %s", got)
	}
	m.update(tea.MouseMsg{X: xOf(cActive), Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.cols[cActive].cursor != 3 {
		t.Fatalf("wheel: cursor %d", m.cols[cActive].cursor)
	}
}

func TestEnvSwitchAndInspect(t *testing.T) {
	e, scans := fixture(t)
	other := filepath.Join(e.Root, "Second", "RimWorld")
	e.CopyTemplate("core", filepath.Join(other, "Data", "Core"))
	e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
	e.MustRun("env", "add", "second", "--game", other)
	tm := start(t, e, scans, 120, 30)
	waitFor(t, tm, " main ")
	keys(tm, "g", "enter") // the full-screen detail, then tab: the technical pages
	waitFor(t, tm, "ludeon.rimworld")
	keys(tm, "tab")
	waitFor(t, tm, "package: ludeon.rimworld")
	keys(tm, "esc", "esc", "E", "down", "enter")
	waitFor(t, tm, "switched to second")
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
	waitAll(t, tm, "已启用 3", "未启用 2")
	keys(tm, "?")
	waitFor(t, tm, "四列")
	keys(tm, "esc", "h", "space")
	waitFor(t, tm, "启用了 Beta")
	m := finish(t, tm)
	for _, l := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(l); w > 100 {
			t.Fatalf("line wider than the terminal (%d): %q", w, l)
		}
	}
	// Command output stays English (05 §1).
	if out, _ := cmds.Execute(e.App, []string{"status"}); !strings.Contains(out, "lineages:") {
		t.Fatalf("status output changed: %s", out)
	}
}

func TestProblemsView(t *testing.T) {
	e, scans := fixture(t)
	// X and Y declare loadAfter each other: an order.cycle whose first suggestion is the
	// ignore rule; Alpha's sample patch has no <value> and fails.
	for _, p := range [][2]string{{"X", "tester.y"}, {"Y", "tester.x"}} {
		m := testkit.Sample("tester."+strings.ToLower(p[0]), p[0])
		dir := filepath.Join(e.Game, "Mods", p[0])
		e.WriteMod(dir, m)
		e.WriteFile(filepath.Join(dir, "About", "About.xml"), strings.Replace(m.AboutXML(), "</ModMetaData>",
			"<loadAfter><li>"+p[1]+"</li></loadAfter></ModMetaData>", 1))
	}
	e.MustRun("scan")
	e.MustRun("enable", "pkg:tester.x", "pkg:tester.y")
	tm := start(t, e, scans, 140, 40)
	waitFor(t, tm, "Enabled 5")
	keys(tm, "!")
	waitFor(t, tm, "patch.no-match")
	time.Sleep(200 * time.Millisecond)
	fm := finish(t, tm)
	ci := -1
	for i, p := range fm.prob.list {
		if p.Code == "order.cycle" {
			ci = i
		}
	}
	if ci < 0 {
		t.Fatalf("no order.cycle in %d problems", len(fm.prob.list))
	}
	if fm.prob.never || fm.prob.stale {
		t.Fatal("analysis did not complete")
	}
	// The rows of X and Y carry the error icon, and the top bar counts them.
	if fm.needCount() == 0 {
		t.Fatal("needs attention is 0")
	}
	// Second session: the analysis is current; select the cycle, expand, a, y.
	tm = start(t, e, scans, 140, 40)
	waitFor(t, tm, "Enabled 5")
	keys(tm, "!")
	waitFor(t, tm, "order.cycle")
	for i := 0; i < ci; i++ {
		keys(tm, "down")
	}
	keys(tm, "enter")
	waitFor(t, tm, "[a] archo rule add ignore decl:")
	keys(tm, "a")
	waitFor(t, tm, "run this command?")
	keys(tm, "y")
	waitFor(t, tm, "r:1 added")
	keys(tm, "i") // ignore whatever is under the cursor now
	time.Sleep(200 * time.Millisecond)
	keys(tm, "esc")
	fm = finish(t, tm)
	if out := e.MustRun("rule", "list"); !strings.Contains(out, "ignore") {
		t.Fatalf("rule not added: %s", out)
	}
	if fm.prob.ignored != 1 {
		t.Fatalf("ignored = %d", fm.prob.ignored)
	}
	for _, p := range fm.prob.list {
		if p.Code == "order.cycle" {
			t.Fatal("cycle still reported after the ignore rule")
		}
	}
}

// 06 §5: the Changes view groups the scan's events, enter shows the file diff, an external
// ModsConfig.xml change offers load / ignore.
func TestChangesView(t *testing.T) {
	e, scans := fixture(t)
	e.WriteFile(filepath.Join(e.Game, "Mods", "B", "Defs", "ThingDefs", "beta.xml"), "<Defs><ThingDef><defName>beta_Changed</defName></ThingDef></Defs>\n")
	e.WriteFile(filepath.Join(e.Game, "Mods", "B", "Patches", "new.xml"), "<Patch/>\n")
	e.Advance(time.Hour)
	e.MustRun("scan")
	e.MustRun("enable", "pkg:tester.b")                                         // an unsaved edit …
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony") // … and an external change
	tm := start(t, e, scans, 120, 30)
	waitFor(t, tm, "Enabled")
	keys(tm, "c")
	waitAll(t, tm, "L load into the workspace", "scan:2", "updated")
	keys(tm, "down", "enter") // the conflict row is selected first; the updated event is next
	waitAll(t, tm, "~ defs/thingdefs/beta.xml", "+ patches/new.xml")
	keys(tm, "up", "I") // back to the conflict row
	waitFor(t, tm, "external ModsConfig.xml version ignored")
	keys(tm, "esc")
	m := finish(t, tm)
	st, _ := e.App.Loadout()
	if st.WS.Conflict != 0 || m.tab != tabList {
		t.Fatal("I did not dismiss the conflict")
	}
}

// 06 §4 / 03 §6.3: log problems sit under one heading per log; a stale log is one folded,
// dimmed row and its problems are not in the counts.
func TestProblemsViewLogs(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	scans := new(int)
	tm := start(t, e, scans, 160, 50)
	keys(tm, "!")
	waitAll(t, tm, "Player.log 1.6.4633", "current (4)", "log.harmony-stack")
	time.Sleep(200 * time.Millisecond)
	fm := finish(t, tm)
	logs := 0
	for _, p := range fm.prob.list {
		if p.Log != nil {
			logs++
		}
	}
	if logs != 4 || fm.prob.counts[0] != 3 {
		t.Fatalf("current log: %d log rows, %d errors", logs, fm.prob.counts[0])
	}

	e.Advance(time.Hour)
	e.MustRun("disable", "pkg:tester.manual")
	tm = start(t, e, scans, 160, 50)
	keys(tm, "!")
	waitAll(t, tm, "stale: older than the Loadout's last change (4)", "log.stale")
	time.Sleep(200 * time.Millisecond)
	fm = finish(t, tm)
	logs = 0
	for _, p := range fm.prob.list {
		if p.Log != nil {
			logs++
			if p.Ref != staleLogRef {
				t.Fatalf("stale log row not folded: %s", p.Ref)
			}
		}
	}
	if logs != 1 || fm.prob.counts[0] != 1 {
		t.Fatalf("stale log: %d log rows, %d errors", logs, fm.prob.counts[0])
	}
}

// 06 §3: every icon of both sets is one cell wide for RimArcho's width function.
func TestIconWidths(t *testing.T) {
	for name, set := range map[string]icons{"nerd": nerdIcons, "compat": compatIcons} {
		for i, s := range []string{set.official, set.workshop, set.local, set.localCopy, set.git, set.err, set.warn, set.logErr,
			set.unloadable, set.unsaved, set.lock, set.update, set.dep, set.staging, set.play, set.save, set.sort, set.search, set.fail,
			markSel, markOK, markPrompt, markBusy, markDot} {
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
	for _, tc := range []struct {
		in   string
		w    int
		want string
	}{
		{"staging is empty · t puts the selected mod here", 20, "staging is empty · t|puts the selected|mod here"},
		{"暂存是空的 · 按 t 把选中的 Mod 放进来", 12, "暂存是空的 ·|按 t|把选中的 Mod|放进来"},
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

func TestParseQuery(t *testing.T) {
	got := parseQuery(`har -tag:ui author:"Brrainz X" id:#abc type:workshop bogus:x -`)
	want := []term{{"", "har", false}, {"tag", "ui", true}, {"author", "brrainz x", false}, {"id", "#abc", false}, {"type", "workshop", false}, {"", "bogus:x", false}, {"", "-", false}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

// 06 §5.5 on the fixture: every field, negation, AND, the per-column query, the fallback.
func TestColumnSearch(t *testing.T) {
	e, _ := fixture(t)
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
		for k := range m.cols {
			m.cols[k].filter = q
			m.refilterCol(colKind(k), "")
		}
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
		m.content = nil // each query loads what it needs itself
		if got := all(q); got != want {
			t.Errorf("%q: got %q, want %q", q, got, want)
		}
	}
	// id:#ref
	ref := m.cols[cActive].items[1].ref
	if got := all("id:" + ref); got != "Alpha | " {
		t.Errorf("id:%s: %q", ref, got)
	}
	// The query stays with its column until esc there; the header shows N/M and the query.
	all("")
	m.focus = cInactive
	press(m, "/")
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tag:core-lib")})
	press(m, "enter")
	press(m, "l") // to "enabled": its own query is empty
	if names(cActive) != "Core,Alpha,Harmony" || names(cInactive) != "Beta" {
		t.Fatalf("per column: %q / %q", names(cActive), names(cInactive))
	}
	title := m.columnTitle(&m.cols[cInactive], false, 40)
	if !strings.Contains(title, "Disabled 1/2") || !strings.Contains(title, "tag:core-lib") {
		t.Fatalf("title %q", title)
	}
	press(m, "esc") // in "enabled": nothing to clear there
	if names(cInactive) != "Beta" {
		t.Fatal("esc cleared another column's query")
	}
	press(m, "h")
	press(m, "esc")
	if names(cInactive) == "Beta" {
		t.Fatal("esc did not clear the query")
	}
}
