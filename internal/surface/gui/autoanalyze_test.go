package gui

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// settle waits until no needs refresh or analysis is running.
func settle(s *API) {
	for {
		s.needsJobs.Wait()
		s.mu.Lock()
		idle := !s.busy["analyze"] && !s.needsRefreshing
		s.mu.Unlock()
		if idle {
			return
		}
	}
}

// 16 §8.9: the files' check starts by itself once per inputs. Repeated visits, a check that
// leaves the list stale, and edits in the editor never chain checks; check again (Analyze)
// always runs, and a repeat while one runs joins it.
func TestAutoAnalysisRunsOncePerInputs(t *testing.T) {
	t.Parallel()
	e, s, _ := fixture(t)
	inner := e.App.AnalyzeRunner
	var runs atomic.Int32
	noop := false
	e.App.AnalyzeRunner = func(env string, p func(string)) error {
		runs.Add(1)
		if noop {
			return nil // a check that leaves the list as it was (the player's loop)
		}
		return inner(env, p)
	}
	// first visit: never checked, so it checks once
	s.Needs()
	settle(s)
	if got := runs.Load(); got != 1 {
		t.Fatalf("first visit ran %d checks, want 1", got)
	}
	for i := 0; i < 5; i++ {
		if n := s.Needs(); n.Never || n.Stale || n.Checked == "" {
			t.Fatalf("after the check: never=%v stale=%v checked=%q", n.Never, n.Stale, n.Checked)
		}
	}
	settle(s)
	if got := runs.Load(); got != 1 {
		t.Fatalf("unchanged visits ran %d checks, want 1", got)
	}
	// edits in the editor while Can it run is not shown: stale, no check by itself
	s.Leave("needs")
	b := s.Board()
	a, _ := b.St.Ix.ResolveOne("pkg:tester.a")
	for i := 0; i < 3; i++ {
		if _, err := s.Toggle(int(view.Active), []string{a.ID}, nil); err != nil {
			t.Fatal(err)
		}
		settle(s)
		if _, err := s.Toggle(int(view.Inactive), []string{a.ID}, nil); err != nil {
			t.Fatal(err)
		}
		settle(s)
	}
	if _, err := s.Toggle(int(view.Active), []string{a.ID}, nil); err != nil {
		t.Fatal(err)
	}
	settle(s)
	if got := runs.Load(); got != 1 {
		t.Fatalf("editor edits ran %d checks, want none", got-1)
	}
	// Can it run shown with a stale check: once for these inputs, even when the check
	// leaves them stale
	noop = true
	for i := 0; i < 4; i++ {
		s.Needs()
		settle(s)
	}
	if got := runs.Load(); got != 2 {
		t.Fatalf("stale visits ran %d checks, want 1", got-1)
	}
	if n := s.Needs(); !n.Stale {
		t.Fatal("the no-op check should have left the list stale")
	}
	// check again right after a check of the same inputs starts nothing
	if s.Analyze() {
		t.Fatal("check again just after a check of the same inputs started another")
	}
	// later it runs; repeats while it runs are refused, and right after it too (one run)
	s.mu.Lock()
	s.analyzedAt = time.Time{}
	s.mu.Unlock()
	release := make(chan struct{})
	e.App.AnalyzeRunner = func(string, func(string)) error { runs.Add(1); <-release; return nil }
	started := 0
	for i := 0; i < 10; i++ {
		if s.Analyze() {
			started++
		}
	}
	close(release)
	settle(s)
	for i := 0; i < 5; i++ {
		if s.Analyze() {
			started++
		}
	}
	settle(s)
	if started != 1 || runs.Load() != 3 {
		t.Fatalf("10 clicks started %d checks (runs %d), want 1", started, runs.Load()-2)
	}
}

// 16 §8.9: page reads made while the same read is on its way join it.
func TestPageReadsJoinTheOneOnItsWay(t *testing.T) {
	t.Parallel()
	_, s, _ := fixture(t)
	gate := make(chan struct{})
	var runs atomic.Int32
	read := func() (int, error) { runs.Add(1); <-gate; return 7, nil }
	out := make(chan int, 10)
	go func() { v, _ := join(&s.flight, "k", read); out <- v }()
	for s.flight.started("k") == 0 {
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 9; i++ {
		go func() { v, _ := join(&s.flight, "k", read); out <- v }()
	}
	time.Sleep(20 * time.Millisecond)
	close(gate)
	for i := 0; i < 10; i++ {
		if v := <-out; v != 7 {
			t.Fatalf("joined read got %d", v)
		}
	}
	if runs.Load() != 1 {
		t.Fatalf("10 reads ran %d times, want 1", runs.Load())
	}
	// after it ended, a read runs again (no stale answer kept)
	if v, _ := join(&s.flight, "k", func() (int, error) { return 8, nil }); v != 8 {
		t.Fatal("a read after the joined one did not run")
	}
	// real page reads: concurrent Settings and Packs calls start one read each burst
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() { s.Settings(); s.Packs(); done <- struct{}{} }()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if n := s.flight.started("Settings"); n < 1 || n > 8 {
		t.Fatalf("Settings reads %d", n)
	}
}
