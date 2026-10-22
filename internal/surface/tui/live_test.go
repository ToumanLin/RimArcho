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

// 06 §2.1: while the game runs the top bar says so and its errors lead their section; when it
// exits both go.
func TestLiveGame(t *testing.T) {
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
	bar := ansi.Strip(m.topBar())
	if want := Tf("game running since %s · %d errors", since, m.live.Errors); !strings.Contains(bar, want) {
		t.Fatalf("top bar %q, want %q", bar, want)
	}
	// this run leads its section, 上次运行的报错 (06 §2.4)
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
	if bar := ansi.Strip(m.topBar()); strings.Contains(bar, Tf("game running since %s", since)) {
		t.Fatalf("still running: %q", bar)
	}
	for _, it := range m.prob.items {
		if it.group == view.GroupThisRun {
			t.Fatalf("this-run item after exit: %+v", it)
		}
	}
}

// 06 §2.5: empty staging shows the history; enter on a step goes there, on a version
// asks first.
func TestHistoryArea(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	e.MustRun("disable", "pkg:tester.a")
	m.reload()
	press(m, "3")
	s := ansi.Strip(m.View())
	step := Tf("disabled %s", "Alpha")
	for _, want := range []string{T("history"), Tf("now · %d not written", 1), step} {
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
	// A saved version: one enter loads it (one step, u brings the list back; 06 §6).
	press(m, "end")
	h := m.hist[m.histCur]
	if h.Kind != "version" {
		t.Fatalf("no version at the end: %+v", m.hist)
	}
	press(m, "enter")
	if want := Tf("loaded v%d into the list (not saved yet)", h.Version) + " · " + T("u undoes"); m.armed != nil || m.status != want {
		t.Fatalf("status %q, want %q", m.status, want)
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
