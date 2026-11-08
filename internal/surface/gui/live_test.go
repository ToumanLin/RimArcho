package gui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// The window starts while a game started elsewhere already runs, and nothing in the
// workspace changed since the last start (VM round 1.0.6, B2): the start-up read adopts the
// board, the Monitor check invalidates it, and the page's next read is served from the
// unchanged workspace snapshot. Live must still see the game, and notice when it exits.
func TestLiveSeesGameRunningBeforeWindowStart(t *testing.T) {
	t.Parallel()
	e, s, events := fixture(t)
	s.needsClosed = true // no background needs refresh: the workspace stays as it is
	running := true
	exe := filepath.Join(e.Game, "RimWorldWin64.exe")
	e.App.GameProbe = func(root string) *app.GameRun {
		if !running || root != e.Game {
			return nil
		}
		return &app.GameRun{PID: 4242, Exe: exe, Started: time.Now().Add(-5 * time.Minute)}
	}
	read := func(request uint64) {
		t.Helper()
		if !s.Workspace(request) {
			t.Fatal("workspace rejected")
		}
		if r := environmentResult(t, events, request); r.Error != "" || r.Stale {
			t.Fatalf("workspace: %+v", r)
		}
	}
	read(1) // the start-up read
	if l, err := s.Live(); err != nil || !l.Running {
		t.Fatalf("after the start-up read: %+v %v", l, err)
	}
	// checkMonitor's refresh (a plan without Harmony is reported, nothing changed) drops
	// the board and sends "scan"; the page reads the workspace again.
	s.mu.Lock()
	s.board, s.needs = nil, nil
	s.mu.Unlock()
	if l, err := s.Live(); err != nil || !l.Running {
		t.Fatalf("between the invalidation and the next read: %+v %v", l, err)
	}
	read(2)
	s.mu.Lock()
	adopted := s.board != nil
	s.mu.Unlock()
	if !adopted {
		t.Fatal("the snapshot read did not give the window its board back")
	}
	if l, err := s.Live(); err != nil || !l.Running {
		t.Fatalf("after the snapshot read: %+v %v", l, err)
	}
	// The game exits: the next poll says so without any page read in between.
	running = false
	if l, err := s.Live(); err != nil || l.Running {
		t.Fatalf("after the exit: %+v %v", l, err)
	}
}

// Only a Monitor check that changed something refreshes the page: the lines naming the
// plans left alone come on every start.
func TestMonitorChangedIgnoresPlansLeftAlone(t *testing.T) {
	t.Parallel()
	for out, want := range map[string]bool{
		"monitor  up to date: 0.11.0 (C:/x)\nmonitor  plan:E3: not added: no Harmony in the plan; Archo Monitor needs it": false,
		"monitor  up to date: 0.11.0 (C:/x)":                                                                 false,
		"monitor  a game is running; the monitor folder is replaced the next time":                           false,
		"monitor  installed 0.11.0 to C:/x":                                                                  true,
		"monitor  replaced C:/x (a development build changed)":                                               true,
		"monitor  plan:default: added after Harmony (one undoable edit)":                                     true,
		"monitor  up to date: 0.11.0 (C:/x)\nmonitor  plan:default: added after Harmony (one undoable edit)": true,
	} {
		if got := monitorChanged(out); got != want {
			t.Errorf("monitorChanged(%q) = %v, want %v", out, got, want)
		}
	}
}
