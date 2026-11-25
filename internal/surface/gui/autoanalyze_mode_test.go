//go:build windows || cgo

package gui

import (
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 16 §8.9, §2.14: automatic checks run in the background mode (low priority, two threads);
// check again does not. analysis.auto "save" checks on Save and on entering Can it run,
// not after edits; "off" checks only on the button.
func TestAutoAnalysisModes(t *testing.T) {
	t.Parallel()
	e, s, _ := fixture(t)
	s.mu.Lock()
	s.analyzeSettle = 100 * time.Millisecond
	s.mu.Unlock()
	inner := e.App.AnalyzeRunner
	var mu sync.Mutex
	var runs []bool // background flag of each check
	e.App.AnalyzeRunner = func(from *app.App, p func(string)) error {
		mu.Lock()
		runs = append(runs, from.AnalyzeBackground)
		mu.Unlock()
		return inner(from, p)
	}
	got := func() []bool {
		mu.Lock()
		defer mu.Unlock()
		return append([]bool(nil), runs...)
	}
	// each step enables one more disabled Mod, so every list is one not checked yet
	// (each enabled set keeps its own check, app.Analysis)
	ix := s.Board().St.Ix
	var off []string
	for _, ref := range []string{"pkg:tester.b", "pkg:brrainz.harmony"} {
		m, _ := ix.ResolveOne(ref)
		off = append(off, m.ID)
	}
	step := 0
	toggle := func() {
		t.Helper()
		col, id := view.Inactive, off[step%2] // the column the Mod is in now
		if step >= 2 {
			col = view.Active
		}
		step++
		if _, err := s.Toggle(int(col), []string{id}, nil); err != nil {
			t.Fatal(err)
		}
	}
	// quiet waits out a settle that would start a check
	quiet := func() {
		settle(s)
		time.Sleep(300 * time.Millisecond)
		settle(s)
	}
	set := func(v string) {
		t.Helper()
		e.MustRun("config", "set", "analysis.auto", v)
	}

	// default "edit": entering Can it run and an edit both check, in the background mode
	s.Needs()
	settle(s)
	s.Leave("needs")
	s.Board()
	toggle()
	quiet()
	if r := got(); len(r) != 2 || !r[0] || !r[1] {
		t.Fatalf("edit mode: checks %v, want two background checks", r)
	}
	// check again is the player's: normal priority
	s.mu.Lock()
	s.analyzedKey = ""
	s.mu.Unlock()
	if !s.Analyze() {
		t.Fatal("check again started nothing")
	}
	settle(s)
	if r := got(); len(r) != 3 || r[2] {
		t.Fatalf("check again: checks %v, want a third at normal priority", r)
	}

	// "save": an edit waits; entering Can it run checks; Save checks
	set("save")
	toggle()
	quiet()
	if r := got(); len(r) != 3 {
		t.Fatalf("save mode: an edit ran %d checks, want 0", len(r)-3)
	}
	s.Needs()
	settle(s)
	s.Leave("needs")
	if r := got(); len(r) != 4 || !r[3] {
		t.Fatalf("save mode: entering Can it run ran %v, want one background check", r[3:])
	}
	toggle()
	quiet()
	if _, err := s.Run(view.ListSave); err != nil {
		t.Fatal(err)
	}
	settle(s)
	if r := got(); len(r) != 5 || !r[4] {
		t.Fatalf("save mode: Save ran %v, want one background check", r[4:])
	}

	// "off": nothing by itself, not on entering Can it run nor on Save
	set("off")
	toggle()
	quiet()
	s.Needs()
	settle(s)
	s.Leave("needs")
	if _, err := s.Run(view.ListSave); err != nil {
		t.Fatal(err)
	}
	quiet()
	if r := got(); len(r) != 5 {
		t.Fatalf("off: %d checks started by themselves, want 0", len(r)-5)
	}
	if n := s.Needs(); !n.Stale && !n.Never {
		t.Fatal("off: the findings should stay stale until check again")
	}
}
