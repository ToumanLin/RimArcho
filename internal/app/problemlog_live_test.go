package app_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// While the game writes Player.log, that log is this run: the problems take the last run
// from Player-prev.log, also when the player had selected Player.log, and go back to
// Player.log when the game exits (06 §4; VM run 2026-10-28 listed one run twice).
func TestProblemLogSkipsTheRunningGamesLog(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	text := e.HarmonyLogFixture()
	running := false
	e.App.GameProbe = func(string) *app.GameRun {
		if running {
			return &app.GameRun{PID: 7, Started: e.Now().Add(-10 * time.Minute)}
		}
		return nil
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	cur, _ := e.App.LogPaths(st)
	prev := cur[:len(cur)-len("Player.log")] + "Player-prev.log"
	e.WriteFile(prev, text)
	os.Chtimes(prev, e.Now().Add(-20*time.Minute), e.Now().Add(-20*time.Minute))
	logOf := func() string {
		t.Helper()
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(ps.Logs) != 1 {
			t.Fatalf("logs %+v", ps.Logs)
		}
		return ps.Logs[0].Path
	}
	if got := e.App.ProblemLog(st); got != cur || logOf() != cur {
		t.Fatalf("no game running: %s", got)
	}
	// the player looked at it before launching again
	if err := e.App.SelectLog(st, cur); err != nil {
		t.Fatal(err)
	}
	e.RecordLaunch(e.Now().Add(-10 * time.Minute))
	running = true
	if e.App.LiveLog(st) != cur {
		t.Fatalf("live log %q", e.App.LiveLog(st))
	}
	if got := e.App.ProblemLog(st); got != prev || logOf() != prev {
		t.Fatalf("running: %s", got)
	}
	// A clean live log must not make the cached previous report look like this run.
	e.WriteFile(cur, "synthetic clean live log\n")
	out := e.MustRun("status")
	for _, want := range []string{"game: running plan:default", "log findings are from a previous run", "log: previous run: Player-prev.log", "current run: archo log --follow"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	// cache-only reads (the sidebar's count): the selected log is the live one, so the run
	// before it
	e.App.LazyLog = true
	if got := e.App.ProblemLog(st); got != prev {
		t.Fatalf("lazy, running: %s", got)
	}
	e.App.LazyLog = false
	running = false
	if got := e.App.ProblemLog(st); got != cur || logOf() != cur {
		t.Fatalf("after exit: %s", got)
	}
	if out := e.MustRun("status"); strings.Contains(out, "log: previous run:") || strings.Contains(out, "log findings are from a previous run") {
		t.Fatalf("labels survived game exit:\n%s", out)
	}
}
