package app_test

import (
	"os"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// While the game writes Player.log, that log is this run: the problems take the last run
// from Player-prev.log, also when the player had selected Player.log, and go back to
// Player.log when the game exits (03 §6.4; VM run 2026-10-28 listed one run twice).
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
	running = true
	if e.App.LiveLog(st) != cur {
		t.Fatalf("live log %q", e.App.LiveLog(st))
	}
	if got := e.App.ProblemLog(st); got != prev || logOf() != prev {
		t.Fatalf("running: %s", got)
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
}
