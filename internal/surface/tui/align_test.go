package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The TUI's pages and keys the window has too (06 §9.1): Search files, H, F, 4 on a narrow
// screen, and the window's names.

func screen(m *model) string { return ansi.Strip(m.View()) }

// Search files: all three fields accept text; options restart the shared search.
func TestFileSearchPage(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	runAlias(m, "search files")
	if m.tab != tabSearch || m.fs.files != view.FileSearchDefaultFiles || !strings.HasPrefix(m.fs.mods, `lists:"`) {
		t.Fatalf("defaults: %+v", m.fs)
	}
	key(m, runes("alpha_Thing?q"))
	if m.fs.text != "alpha_Thing?q" || m.quitting || m.panel != nil {
		t.Fatalf("typing: %q", m.fs.text)
	}
	key(m, tea.KeyMsg{Type: tea.KeyBackspace})
	key(m, tea.KeyMsg{Type: tea.KeyBackspace})
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.fs.running || m.fs.page == nil {
		t.Fatalf("running %v page %+v", m.fs.running, m.fs.page)
	}
	text := screen(m)
	for _, want := range []string{"mod library › search files", "Alpha", "Defs/ThingDefs/alpha.xml", "alpha_Thing"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s:\n%s", want, text)
		}
	}
	if row := m.fs.rows[m.fs.cursor]; !row.cursor || row.file < 0 {
		t.Fatalf("cursor %+v", row)
	}
	key(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.fs.row != 1 || m.fs.results {
		t.Fatal("down did not reach files")
	}
	key(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	key(m, runes("patches"))
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.fs.rows) != 0 || !strings.Contains(screen(m), "not found") {
		t.Fatalf("patches:\n%s", screen(m))
	}
	key(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	key(m, runes("De"))
	key(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.fs.files != "Defs" {
		t.Fatalf("files completion %q", m.fs.files)
	}
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r"), Alt: true})
	if !m.fs.regex || len(m.fs.rows) == 0 {
		t.Fatal("regex did not rerun")
	}
	key(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.fs.row != 2 {
		t.Fatal("down did not reach mods")
	}
	key(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m.fs.text = "beta_Thing"
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(screen(m), "Beta") {
		t.Fatalf("blank mods did not include disabled copy:\n%s", screen(m))
	}
	key(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	key(m, runes("name:Alpha"))
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.fs.rows) != 0 {
		t.Fatal("library query did not scope copies")
	}
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.tab != tabList {
		t.Fatalf("esc ×3: %d", m.tab)
	}
}

// Leaving Search files cancels its search (06 §3.1 rule 9).
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

func TestFileSearchEscapeStopsAndKeepsResults(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	m.openSearch()
	m.fs.text = "alpha_Thing"
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	count := len(m.fs.rows)
	if count == 0 {
		t.Fatal("no initial results")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.fs.cancel = cancel
	m.fs.running = true
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	if ctx.Err() == nil || m.fs.running || len(m.fs.rows) != count {
		t.Fatal("escape did not preserve stopped results")
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
	for _, w := range []string{"mod library › updates", "2 waiting for you to update", "3 already updated", "a every mod"} {
		if !strings.Contains(s, w) {
			t.Fatalf("Updates has no %q:\n%s", w, s)
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
	// Background tasks is the shared record as text, one line a task under its group
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

// Load-order rules (06 §9.1): r opens it with the form for the cursor's mod; the other mod comes from
// the candidates; the rule is listed under only the current list; Del removes it, u puts it back with
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

// F on Can it run enables the installed requirement and sorts; the check Can it run then starts
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

// Start (06 §2.3, §9.1): shift+1 and the editor's L open it; it shows the current list and
// ▶ launch game, and only L there launches.
func TestStartPage(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	key(m, runes("!"))
	s := screen(m)
	if m.tab != tabHome || !strings.Contains(s, "default") || !strings.Contains(s, T("launch the game")) || !strings.Contains(s, "› "+T("start")) {
		t.Fatalf("start page:\n%s", s)
	}
	press(m, "esc")
	if m.tab != tabList {
		t.Fatalf("esc: tab %d", m.tab)
	}
	key(m, runes("L"))
	if m.tab != tabHome || len(e.Launched) != 0 {
		t.Fatalf("editor L: tab %d, launched %v", m.tab, e.Launched)
	}
	key(m, runes("L"))
	if len(e.Launched) != 1 {
		t.Fatalf("L on Start did not launch: %v (%s)", e.Launched, m.status)
	}
}

// Lists are opened, not switched to (04 §1.3): P opens another list, the top bar says which
// one launches, make current or the editor's L makes it current.
func TestOpenListIsNotCurrent(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	e.MustRun("plan", "new", "second")
	m := direct(t, e, 160, 40)
	envID := m.st.Ix.Env.ID
	cur := e.App.Store.CurrentPlanID(envID)
	pick := func(prefix string) {
		key(m, runes("P"))
		for i, c := range m.panel.choices {
			if strings.HasPrefix(c, prefix) {
				m.panel.choice = i
				key(m, tea.KeyMsg{Type: tea.KeyEnter})
				return
			}
		}
		t.Fatalf("no %q in %q", prefix, m.panel.choices)
	}
	pick("second")
	if m.st.Plan.Name != "second" || e.App.Store.CurrentPlanID(envID) != cur || m.launches == "" {
		t.Fatalf("open: editing %s, current %d (was %d), launches %q", m.st.Plan.Name, e.App.Store.CurrentPlanID(envID), cur, m.launches)
	}
	if top := strings.Split(screen(m), "\n")[0]; !strings.Contains(top, Tf("launching plays %s", m.launches)) {
		t.Fatalf("top bar: %q", top)
	}
	pick(T("make current"))
	if m.launches != "" || e.App.Store.CurrentPlanID(envID) == cur {
		t.Fatalf("make current: launches %q", m.launches)
	}
	pick("default")
	if m.launches != "second" {
		t.Fatalf("open default: launches %q", m.launches)
	}
	key(m, runes("L")) // launching from its editor makes it current
	if m.launches != "" || m.tab != tabHome || e.App.Store.CurrentPlanID(envID) != cur {
		t.Fatalf("L: launches %q tab %d", m.launches, m.tab)
	}
}

// T (06 §3.5): in staging everything goes back; elsewhere every enabled mod is staged.
func TestStageAllKey(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	m.focus = cActive
	press(m, "T")
	if len(m.cols[cStaging].items) == 0 {
		t.Fatalf("T in Enabled staged nothing: %s", m.status)
	}
	m.focus = cStaging
	press(m, "T")
	if len(m.cols[cStaging].items) != 0 || activeNames(t, e) != "Core, Alpha, Harmony" {
		t.Fatalf("T in staging: %d left, %s", len(m.cols[cStaging].items), activeNames(t, e))
	}
}
