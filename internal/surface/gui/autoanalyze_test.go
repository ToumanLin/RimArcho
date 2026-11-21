package gui

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// settle waits until no automatic check is pending and no needs refresh or analysis runs.
func settle(s *API) {
	for {
		s.needsJobs.Wait()
		s.mu.Lock()
		idle := !s.busy["analyze"] && !s.needsRefreshing && s.analyzeTimer == nil
		s.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// 16 §8.9: the shown findings stay current with at most one check per settled state. A
// list never checked checks on entry; a run of edits ends in one check of the final list;
// an edit during a check gets one check after it; Launch starts one too. The same inputs
// never start by themselves twice (a check that leaves the list stale does not loop), and
// check again right after a check of the same inputs starts nothing.
func TestAutoAnalysisKeepsFindingsCurrent(t *testing.T) {
	t.Parallel()
	e, s, _ := fixture(t)
	s.mu.Lock()
	s.analyzeSettle = 300 * time.Millisecond
	s.mu.Unlock()
	inner := e.App.AnalyzeRunner
	var runs atomic.Int32
	var noop atomic.Bool
	// hold: a check that has read its list waits (ran says so) until release closes
	type held struct{ ran, release chan struct{} }
	var hold atomic.Pointer[held]
	e.App.AnalyzeRunner = func(env string, p func(string)) error {
		runs.Add(1)
		var err error
		if !noop.Load() { // a no-op check leaves the list as it was (the player's loop)
			err = inner(env, p)
		}
		if h := hold.Load(); h != nil {
			close(h.ran)
			<-h.release
		}
		return err
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
	// a run of edits in the editor, Can it run not shown: one check of the final list
	s.Leave("needs")
	b := s.Board()
	// each enabled set keeps its own check (app.Analysis), so every step makes a set not
	// checked yet; a, b, h: tester.a (enabled), tester.b and Harmony (disabled)
	id := func(ref string) string {
		m, _ := b.St.Ix.ResolveOne(ref)
		return m.ID
	}
	a, bb, h := id("pkg:tester.a"), id("pkg:tester.b"), id("pkg:brrainz.harmony")
	toggle := func(c view.Column, mod string) {
		t.Helper()
		if _, err := s.Toggle(int(c), []string{mod}, nil); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		toggle(view.Active, a)
		toggle(view.Inactive, a)
	}
	toggle(view.Inactive, bb)
	settle(s)
	if got := runs.Load(); got != 2 {
		t.Fatalf("seven edits ran %d checks, want 1", got-1)
	}
	if n := s.Needs(); n.Stale || n.Never {
		t.Fatal("after the edits settled the findings are not current")
	}
	settle(s)
	// an edit while a check runs: that check ends, then one more for the list as it is
	hd := &held{make(chan struct{}), make(chan struct{})}
	hold.Store(hd)
	toggle(view.Inactive, h)
	<-hd.ran
	hold.Store(nil)
	toggle(view.Active, a)
	time.Sleep(400 * time.Millisecond) // the edit settles while the check is held
	close(hd.release)
	settle(s)
	if got := runs.Load(); got != 4 {
		t.Fatalf("edits during a check ran %d checks, want 2 (the running one and one after)", got-2)
	}
	if n := s.Needs(); n.Stale || n.Never {
		t.Fatal("the check after the running one should leave the findings current")
	}
	settle(s)
	// Launch reads the stored check: it starts one at once when the list changed
	s.mu.Lock()
	s.analyzeSettle = time.Hour // the edit alone would not start it during this test
	s.mu.Unlock()
	toggle(view.Active, bb)
	// while the edit settles, the editor's findings say they are updating (16 §8.7)
	for {
		s.mu.Lock()
		refreshing, pending, work := s.needsRefreshing, s.analyzeTimer != nil, s.needsWorkLocked()
		s.mu.Unlock()
		if !refreshing {
			if !pending || !work {
				t.Fatalf("after an edit: check pending %v, updating %v; want both", pending, work)
			}
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if s.Home() == nil {
		t.Fatal("no Launch page")
	}
	settle(s)
	if got := runs.Load(); got != 5 {
		t.Fatalf("Launch after an edit ran %d checks, want 1", got-4)
	}
	// a check that leaves its inputs stale does not start again by itself
	noop.Store(true)
	toggle(view.Active, h)
	s.Home()
	settle(s)
	for i := 0; i < 4; i++ {
		s.Needs()
		s.Home()
		settle(s)
	}
	if got := runs.Load(); got != 6 {
		t.Fatalf("a stale-leaving check ran %d times by itself, want 1", got-5)
	}
	if n := s.Needs(); !n.Stale {
		t.Fatal("the no-op check should have left the list stale")
	}
	settle(s)
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
	if started != 1 || runs.Load() != 7 {
		t.Fatalf("10 clicks started %d checks (runs %d), want 1", started, runs.Load()-6)
	}
	// the task list keeps one row for the check of this list, updated in place
	rows := 0
	for _, task := range s.tasks.List() {
		if task.Kind == view.TaskAnalyze && task.State == "done" {
			rows++
		}
	}
	if rows != 1 {
		t.Fatalf("%d finished check rows, want 1", rows)
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
