package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The TUI's pages and keys the window has too (06 §9.1): 搜索文件, H, F, 4 on a narrow
// screen, and the window's names.

func screen(m *model) string { return ansi.Strip(m.View()) }

// 搜索文件: reached by tab from 添加订阅 and from the palette; typing goes into the box, enter
// searches the open list's enabled mods, the results are mod → file → line, and the options
// change the search at once.
func TestFileSearchPage(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	runAlias(m, "search files")
	if m.tab != tabSearch {
		t.Fatalf("palette: tab %d", m.tab)
	}
	s := screen(m)
	if !strings.Contains(s, "mod library › search files") || !strings.Contains(s, view.Tf("the %d mods %s enables", 3, m.st.Plan.Name)) {
		t.Fatalf("page:\n%s", s)
	}
	// letters, ? and q are the box's
	key(m, runes("alpha_Thing?q"))
	if m.fs.text != "alpha_Thing?q" || m.quitting || m.panel != nil {
		t.Fatalf("typing: %q quit %v panel %v", m.fs.text, m.quitting, m.panel != nil)
	}
	key(m, tea.KeyMsg{Type: tea.KeyBackspace})
	key(m, tea.KeyMsg{Type: tea.KeyBackspace})
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.fs.running || m.fs.page == nil {
		t.Fatalf("search did not end: running %v page %v status %q", m.fs.running, m.fs.page, m.status)
	}
	s = screen(m)
	if !strings.Contains(s, "Alpha") || !strings.Contains(s, "Defs/ThingDefs/alpha.xml") || !strings.Contains(s, "alpha_Thing") {
		t.Fatalf("results:\n%s", s)
	}
	// the cursor is on the file row, never on the mod's head
	if r := m.fs.rows[m.fs.cursor]; !r.cursor || r.file < 0 {
		t.Fatalf("cursor on %+v", r)
	}
	// ↓ goes to the kind row; ← → and space switch a kind and search again
	key(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.fs.row != 1 || m.fs.results {
		t.Fatalf("↓: row %d results %v", m.fs.row, m.fs.results)
	}
	for i := 0; i < 2; i++ { // past 文件, C# 代码 to Defs
		key(m, tea.KeyMsg{Type: tea.KeyRight})
	}
	key(m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if m.fs.kinds["defs"] {
		t.Fatal("space did not leave out Defs")
	}
	if len(m.fs.rows) != 0 || !strings.Contains(screen(m), "nothing found") {
		t.Fatalf("without Defs:\n%s", screen(m))
	}
	key(m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	// alt+r is the regex, from any row
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r"), Alt: true})
	if !m.fs.regex || len(m.fs.rows) == 0 {
		t.Fatalf("alt+r: regex %v rows %d", m.fs.regex, len(m.fs.rows))
	}
	// C# code wants a named mod first
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2"), Alt: true})
	if !m.fs.code || !strings.Contains(m.status, "named mods") {
		t.Fatalf("alt+2: code %v status %q", m.fs.code, m.status)
	}
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1"), Alt: true})
	// + Mod on the scope row names a mod: typing finds it, enter adds it
	key(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.fs.row != 2 {
		t.Fatalf("↓: row %d", m.fs.row)
	}
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.fs.picking {
		t.Fatal("enter on + Mod did not open the name box")
	}
	key(m, runes("bet"))
	if len(m.fs.cands) == 0 || m.fs.cands[0].Name != "Beta" {
		t.Fatalf("candidates %+v", m.fs.cands)
	}
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.fs.mods) != 1 || m.fs.mods[0].Name != "Beta" || m.fs.picking {
		t.Fatalf("named %+v picking %v", m.fs.mods, m.fs.picking)
	}
	if strings.Contains(screen(m), "Alpha") {
		t.Fatalf("the search still covers the list:\n%s", screen(m))
	}
	// esc: back to the box, the results, then the editor; at most 3 (06 §11 T20)
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.tab != tabList {
		t.Fatalf("esc ×3: tab %d", m.tab)
	}
}

// Leaving 搜索文件 cancels its search (06 §3.1 rule 9).
func TestFileSearchLeaveStops(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	m.openSearch()
	m.fs.text = "x"
	m.fsRun(1) // not drained: it is still running
	if !m.fs.running {
		t.Fatal("not running")
	}
	key(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.tab != tabSubscribe || m.fs.running || m.fs.cancel != nil {
		t.Fatalf("tab %d running %v", m.tab, m.fs.running)
	}
}

// H lists every step and version, and enter goes there; A checks the files; 4 on a screen
// with no detail column opens the full-screen detail (06 §2.5, §3.5).
func TestEditorKeysOfTheWindow(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	press(m, "1")
	press(m, "space") // enable Beta: one step
	if m.unsaved == 0 {
		t.Fatal("space did nothing")
	}
	before := activeNames(t, e)
	press(m, "H")
	if m.panel == nil || len(m.panel.choices) < 2 || m.panel.title != T("history") {
		t.Fatalf("H: %+v", m.panel)
	}
	// down from "now" is the step before the edit
	pressKey(m, tea.KeyDown)
	pressKey(m, tea.KeyEnter)
	if m.panel != nil || activeNames(t, e) == before {
		t.Fatalf("H enter: panel %v, list %s", m.panel != nil, activeNames(t, e))
	}
	// 4 with no detail column
	n := direct(t, e, 100, 30)
	press(n, "2")
	press(n, "4")
	if !n.full {
		t.Fatal("4 on a narrow screen did not open the details")
	}
	// ? in the full-screen detail lists the keys, esc goes back to the detail
	press(n, "?")
	if n.panel == nil || !n.full {
		t.Fatalf("? in the detail: panel %v full %v", n.panel != nil, n.full)
	}
	press(n, "esc")
	if n.panel != nil || !n.full {
		t.Fatal("esc did not go back to the detail")
	}
}

// The window's names on the TUI's pages (06 §2.1, §2.9).
func TestWindowNames(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	if top := strings.Split(screen(m), "\n")[0]; strings.Contains(top, "needs attention") {
		t.Fatalf("top bar: %q", top)
	}
	press(m, "c")
	s := screen(m)
	for _, w := range []string{"mod library › subscriptions and updates", "2 waiting for you to update", "3 already updated", "a every mod"} {
		if !strings.Contains(s, w) {
			t.Fatalf("订阅与更新 has no %q:\n%s", w, s)
		}
	}
	press(m, "^")
	if s := screen(m); !strings.Contains(s, "mod library › all mods") {
		t.Fatalf("library:\n%s", s)
	}
	// every palette command does something in the TUI
	for _, c := range view.Commands {
		if c.ID == "quit" || c.ID == "uninstall" {
			continue
		}
		m := direct(t, e, 160, 40)
		before := screen(m)
		m.palRun(c.ID)(m)
		if after := screen(m); after == before && m.status == "" && m.panel == nil && !m.cmdOn {
			t.Errorf("palette command %s did nothing", c.ID)
		}
	}
	// 后台任务 is the shared record as text, one line a task under its group
	m = direct(t, e, 160, 40)
	runAlias(m, "background tasks")
	if m.panel == nil || m.panel.title != "background tasks" || !strings.Contains(strings.Join(m.panel.lines, "\n"), "nothing has run in the background yet") {
		t.Fatalf("tasks panel: %+v", m.panel)
	}
	press(m, "esc")
	ts := m.taskList()
	ts.Finish(ts.Begin(view.TaskScan, "", nil), app.TaskFailed, "disk full")
	runAlias(m, "background tasks")
	if s := strings.Join(m.panel.lines, "\n"); !strings.Contains(s, "today") || !strings.Contains(s, "failed") || !strings.Contains(s, "disk full") {
		t.Fatalf("tasks panel:\n%s", s)
	}
}

// 排序规则 (06 §9.1): r opens it with the form for the cursor's mod; the other mod comes from
// the candidates; the rule is listed under 只看当前列表; Del removes it, u puts it back with
// its number; enter changes it; letters in the form are the form's.
func TestRulesScreen(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	press(m, "2")
	for m.current() != nil && m.current().name != "Alpha" {
		press(m, "down")
	}
	press(m, "r")
	if m.tab != tabRules || m.rules.form == nil || m.rules.form.a.Name != "Alpha" {
		t.Fatalf("r: tab %d form %+v", m.tab, m.rules.form)
	}
	key(m, runes("harmq"))
	if m.quitting {
		t.Fatal("q in the form quit")
	}
	key(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if len(m.rules.form.cands) == 0 || m.rules.form.cands[0].Name != "Harmony" {
		t.Fatalf("candidates %+v", m.rules.form.cands)
	}
	key(m, tea.KeyMsg{Type: tea.KeyEnter}) // takes Harmony, goes to the note
	if m.rules.form.b == nil || m.rules.form.field != 2 {
		t.Fatalf("pick: %+v", m.rules.form)
	}
	key(m, runes("why"))
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.rules.form != nil || !strings.HasPrefix(m.status, "rule added: Alpha loads after Harmony") {
		t.Fatalf("save: form %v status %q", m.rules.form != nil, m.status)
	}
	rows := m.rules.shown()
	if len(rows) != 1 || rows[0].Note != "why" || !strings.Contains(screen(m), "Alpha loads after Harmony") {
		t.Fatalf("rows %+v\n%s", rows, screen(m))
	}
	id := rows[0].ID
	// all rules / the current list; kept across starts
	key(m, runes("a"))
	if !m.rules.all || e.App.Store.Meta(rulesScopeKey) != "all" {
		t.Fatal("a did not switch to all rules")
	}
	key(m, runes("a"))
	// enter changes it: the kind with ← →
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	key(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	key(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.rules.form.field != 0 {
		t.Fatalf("shift+tab ×2: field %d", m.rules.form.field)
	}
	key(m, tea.KeyMsg{Type: tea.KeyRight})
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.HasPrefix(m.status, "rule changed: Alpha loads before Harmony") || m.rules.shown()[0].ID != id {
		t.Fatalf("change: %q", m.status)
	}
	// Del removes it at once; u right after puts it back with its number
	key(m, tea.KeyMsg{Type: tea.KeyDelete})
	if len(m.rules.data.Rows) != 0 {
		t.Fatalf("Del: %+v", m.rules.data.Rows)
	}
	key(m, runes("u"))
	if rs := m.rules.data.Rows; len(rs) != 1 || rs[0].ID != id {
		t.Fatalf("u: %+v", rs)
	}
	// o shows the rule's mod in the list; esc from the screen is the editor
	key(m, runes("o"))
	if m.tab != tabList || m.current() == nil || m.current().name != "Alpha" {
		t.Fatalf("o: tab %d current %v", m.tab, m.current())
	}
	runAlias(m, "load-order rules")
	if m.tab != tabRules || m.rules.form != nil {
		t.Fatal("the palette did not open the rules")
	}
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.tab != tabList {
		t.Fatalf("esc: tab %d", m.tab)
	}
}

// F on 能不能跑 enables the installed requirement and sorts; the check 能不能跑 then starts
// by itself says so in the head and leaves the status line's undo hint alone (06 §2.4).
func TestAutoFixKeepsTheUndoHint(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	g := testkit.Sample("tester.g", "Gamma")
	g.Deps = []string{"tester.b"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "G"), g)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a", "brrainz.harmony", "tester.g")
	e.MustRun("scan")
	checks := 0
	m := New(Options{App: e.App, Scan: func(string) (string, error) { t.Fatal("scan"); return "", nil },
		Analyze: func(_ string, progress func(string)) error {
			checks++
			progress("progress: defs")
			_, err := e.App.Analyze(nil)
			return err
		}}).(*model)
	m.Init()
	m.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	key(m, runes("@"))
	if n := view.AutoFixCount(m.prob.base); n != 2 {
		t.Fatalf("auto-fix count %d:\n%s", n, screen(m))
	}
	if !strings.Contains(screen(m), "F "+Tf("fix %d automatically", 2)) {
		t.Fatalf("no F in the hints:\n%s", screen(m))
	}
	before := checks
	key(m, runes("F")) // the edit, then the check it starts, run to the end
	if checks == before {
		t.Fatal("no check after the fix")
	}
	if !strings.Contains(m.status, T("u undoes")) {
		t.Fatalf("status %q", m.status)
	}
	if view.AutoFixCount(m.prob.base) != 0 {
		t.Fatalf("still fixable:\n%s", screen(m))
	}
}
