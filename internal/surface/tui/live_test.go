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

// 10 §3.4: while the game runs the top bar says so and its errors lead 需要处理; when it
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
	if len(m.prob.items) == 0 || m.prob.items[0].group != view.GroupThisRun {
		t.Fatalf("needs do not start with this run: %+v", m.prob.items)
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

// 08 §14, 10 §6: empty staging shows the history; enter on a step goes there, on a version
// asks first.
func TestHistoryArea(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	e.MustRun("disable", "pkg:tester.a")
	m.reload()
	press(m, "3")
	s := ansi.Strip(m.View())
	step := Tf("disabled %s", "Alpha")
	for _, want := range []string{T("history"), Tf("now · %d not saved", 1), step} {
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
	// A saved version: the first enter only warns, the second loads it.
	press(m, "G")
	h := m.hist[m.histCur]
	if h.Kind != "version" {
		t.Fatalf("no version at the end: %+v", m.hist)
	}
	press(m, "enter")
	if m.armed == nil || !strings.Contains(m.status, T("load this version into the list")) {
		t.Fatalf("not asked: %q", m.status)
	}
	press(m, "enter")
	if want := Tf("loaded v%d into the list (not saved yet)", h.Version); m.status != want {
		t.Fatalf("status %q, want %q", m.status, want)
	}
	// Staged mods take the place back from the history.
	press(m, "1")
	press(m, "t")
	if m.historyShown() {
		t.Fatal("history shown with mods in staging")
	}
}
