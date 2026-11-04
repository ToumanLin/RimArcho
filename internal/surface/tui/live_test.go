package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 06 §9.1: while the game runs the status line says so and its errors lead their section; when it
// exits both go.
func TestLiveGame(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	running := true
	e.App.GameProbe = func(string) *app.GameRun {
		if !running {
			return nil
		}
		return &app.GameRun{PID: 42, Started: e.Now().Add(-10 * time.Minute)}
	}
	m := direct(t, e, 160, 40)
	m.Update(liveTick{})
	since := e.Now().Add(-10 * time.Minute).In(e.App.Loc).Format("15:04")
	if m.live == nil || !m.live.Running || m.live.Errors == 0 {
		t.Fatalf("live: %+v", m.live)
	}
	// the status line says so, the errors an amber number beside it; the top bar does not
	line := ansi.Strip(m.statusLine())
	if want := "● " + Tf("game running since %s", since) + " " + itoa(m.live.Errors); !strings.HasSuffix(strings.TrimRight(line, " "), want) {
		t.Fatalf("status line %q, want %q at its end", line, want)
	}
	if bar := ansi.Strip(m.topBar()); strings.Contains(bar, Tf("game running since %s", since)) || strings.Contains(bar, "RimArcho  RimArcho") {
		t.Fatalf("top bar %q", bar)
	}
	// a message takes the left of the line; the game stays on the right
	m.say(sayOK, "done")
	if line := ansi.Strip(m.statusLine()); !strings.HasPrefix(line, "✓ done") || !strings.Contains(line, Tf("game running since %s", since)) {
		t.Fatalf("status line with a message %q", line)
	}
	// this run leads its section, errors from the last run (06 §2.4)
	first := -1
	for i, it := range m.prob.items {
		if it.section == view.SectionRun {
			first = i
			break
		}
	}
	if first < 0 || m.prob.items[first].group != view.GroupThisRun {
		t.Fatalf("the run section does not start with this run: %+v", m.prob.items)
	}
	m.tab = tabProblems
	if s := ansi.Strip(m.View()); !strings.Contains(s, T("this run")) {
		t.Fatalf("no this-run group:\n%s", s)
	}
	m.tab = tabList
	running = false
	m.Update(liveTick{})
	if line := ansi.Strip(m.statusLine()); strings.Contains(line, Tf("game running since %s", since)) {
		t.Fatalf("still running: %q", line)
	}
	for _, it := range m.prob.items {
		if it.group == view.GroupThisRun {
			t.Fatalf("this-run item after exit: %+v", it)
		}
	}
}

// 06 §2.5: empty staging shows the history; enter on a step or version goes there
// immediately, and u restores the previous list.
func TestHistoryArea(t *testing.T) {
	e, _ := fixture(t)
	// On the fresh fixture the list is the saved version: going there changes nothing.
	t.Run("identical checkout is a no-op", func(t *testing.T) {
		m := direct(t, e, 160, 40)
		before := *m.st.WS
		depth := e.App.Store.StepCount(m.st.Plan.ID, "undo")
		for i, h := range m.hist {
			if h.Kind == "version" {
				m.histCur = i
				break
			}
		}
		if m.hist[m.histCur].Kind != "version" {
			t.Fatal("no saved version")
		}
		if strings.Contains(m.histLabel(), T("u undoes")) {
			t.Fatal("unconditional undo hint")
		}
		m.histGo()
		if m.status != T("no changes") {
			t.Fatalf("no-op feedback: %s", m.status)
		}
		after, err := e.App.Store.Workspace(m.st.Plan.ID)
		if err != nil {
			t.Fatal(err)
		}
		if *after != before || e.App.Store.StepCount(m.st.Plan.ID, "undo") != depth {
			t.Fatal("identical checkout mutated workspace/history")
		}
	})
	m := direct(t, e, 160, 40)
	e.MustRun("disable", "pkg:tester.a")
	m.reload()
	press(m, "3")
	s := ansi.Strip(m.View())
	step := Tf("disabled %s", "Alpha")
	for _, want := range []string{T("history"), Tf("now  %d unsaved", 1), step} {
		if !strings.Contains(s, want) {
			t.Fatalf("no %q in the history area:\n%s", want, s)
		}
	}
	if m.hist[m.histCur].Kind != "now" {
		t.Fatalf("cursor not on now: %+v", m.hist)
	}
	press(m, "down")
	if h := m.hist[m.histCur]; h.Kind != "undo" || h.Text != step {
		t.Fatalf("under now: %+v", h)
	}
	press(m, "enter")
	if got := activeNames(t, e); !strings.Contains(got, "Alpha") {
		t.Fatalf("Alpha not back: %s", got)
	}
	if m.hist[0].Kind != "redo" || m.hist[0].Text != step {
		t.Fatalf("no redo row: %+v", m.hist)
	}
	if s := ansi.Strip(m.View()); !strings.Contains(s, step) {
		t.Fatalf("redo row not drawn:\n%s", s)
	}
	// Make the current list differ from the saved version, so checkout must do work.
	e.MustRun("disable", "pkg:tester.a")
	m.reload()
	before := m.st.List.Encode()
	steps := e.App.Store.StepCount(m.st.Plan.ID, "undo")
	// A saved version: one enter loads it (one step, u brings the list back; 06 §6).
	press(m, "end")
	h := m.hist[m.histCur]
	if h.Kind != "version" {
		t.Fatalf("no version at the end: %+v", m.hist)
	}
	SetLang("zh")
	t.Cleanup(func() { SetLang("en") })
	press(m, "enter")
	if want := Tf("back at v%d (unsaved)", h.Version) + Tf(", %s", T("u undoes")); m.armed != nil || m.status != want {
		t.Fatalf("status %q, want %q", m.status, want)
	}
	if got := activeNames(t, e); got != "Core, Alpha, Harmony" {
		t.Fatalf("one enter did not load the version: %s", got)
	}
	history, err := e.App.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Undo) != steps+1 || history.Undo[0].Op != "checkout" {
		t.Fatalf("checkout is not one history step: %+v", history.Undo)
	}
	if line := ansi.Strip(m.statusLine()); !strings.Contains(line, "u 可撤销") {
		t.Fatalf("checkout status: %q", line)
	}
	SetLang("en")
	press(m, "u")
	if got := m.st.List.Encode(); got != before {
		t.Fatalf("u did not restore the previous list: %s, want %s", got, before)
	}
	if got := e.App.Store.StepCount(m.st.Plan.ID, "undo"); got != steps {
		t.Fatalf("undo stack after u: %d, want %d", got, steps)
	}
	// Staged mods take the place back from the history.
	press(m, "1")
	press(m, "t")
	if m.historyShown() {
		t.Fatal("history shown with mods in staging")
	}
}

// 08 §1: while another environment's game runs, the top bar shows why launching is off
// instead of the L key.
func TestTopBarLaunchBlocked(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	m := direct(t, e, 160, 40)
	if bar := ansi.Strip(m.topBar()); !strings.Contains(bar, "L "+T("launch")) {
		t.Fatalf("no launch key: %q", bar)
	}
	m.live = &view.Live{Blocked: Tf("a game is already running (%s)", "second")}
	bar := ansi.Strip(m.topBar())
	if !strings.Contains(bar, Tf("a game is already running (%s)", "second")) || strings.Contains(bar, "L "+T("launch")) {
		t.Fatalf("blocked top bar: %q", bar)
	}
}

// G17 in the TUI: with no game running the status line says nothing about it, unless a new
// Player.log waits to be analysed; then it says so and which key opens Can it run.
func TestStatusLineAfterGame(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	m := direct(t, e, 160, 40)
	m.live = &view.Live{}
	if line := strings.TrimSpace(ansi.Strip(m.statusLine())); line != "" {
		t.Fatalf("idle status line %q", line)
	}
	m.live = &view.Live{NewLog: T("analyse this run's log")}
	if line := strings.TrimSpace(ansi.Strip(m.statusLine())); line != "● "+T("analyse this run's log")+"  shift+2" {
		t.Fatalf("new-log status line %q", line)
	}
	press(m, "@")
	if m.tab != tabProblems {
		t.Fatalf("shift+2 went to %d", m.tab)
	}
}
