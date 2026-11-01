package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 06 §2.1: while the game runs, its log's errors become "this run" items; when it exits the
// tracker stops.
func TestLiveTracker(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	text := e.HarmonyLogFixture()
	running := true
	e.App.GameProbe = func(string) *app.GameRun {
		if !running {
			return nil
		}
		return &app.GameRun{PID: 42, Started: e.Now().Add(-10 * time.Minute)}
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	path, _ := e.App.LogPaths(st)
	if path == "" {
		path = filepath.Join(e.UserData, "Player.log")
	}
	os.WriteFile(path, []byte(text), 0o644)
	var tr view.LiveTracker
	l, err := tr.Update(e.App, st)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Running || l.Errors == 0 || l.Items[0].Group != view.GroupThisRun {
		t.Fatalf("running: %+v", l)
	}
	found := false
	for _, it := range l.Items {
		if strings.Contains(it.Text, "500×") {
			found = true
		}
		// the section heading names the run; the count is in the text, not the evidence
		if strings.Contains(it.Text, "this run") || strings.ContainsAny(it.Evidence.Source, "0123456789") {
			t.Fatalf("repeats the run or the count: %+v", it)
		}
	}
	if !found {
		t.Fatalf("no item for the repeated error: %+v", l.Items)
	}
	// "open game log" finds each item's row on the log page by its first line: the live
	// clusters are numbered per batch, so their ids are not the page's (VM run 2026-10-28).
	page, err := view.BuildLogPage(e.App, st, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	// the game still writes it: this run, not yet in Can it run as the last run
	if page.Selected || !page.Live {
		t.Fatalf("log page of the running game's log: selected %v, live %v", page.Selected, page.Live)
	}
	lines := map[string]bool{}
	for _, r := range page.Rows {
		lines[r.Line] = r.Line != ""
	}
	for _, it := range l.Items {
		a := it.Actions[0]
		if a.Kind != view.ActGameLog || a.Cluster != "" || !lines[a.Line] {
			t.Fatalf("item %q: action %+v matches no row", it.Text, a)
		}
	}
	running = false
	if l, _ = tr.Update(e.App, st); l.Running || len(l.Items) != 0 {
		t.Fatalf("after exit: %+v", l)
	}
}

// 08 §1: another environment's game blocks launching; the tracker says so instead of
// following that game's log.
func TestLiveBlockedByOtherEnv(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	other := filepath.Join(e.Root, "Other", "RimWorld")
	e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
	e.WriteFile(filepath.Join(other, "Version.txt"), "1.6.4633 rev1261\n")
	e.MustRun("env", "add", "second", "--game", other)
	e.MustRun("env", "use", "main")
	e.App.GameProbe = func(root string) *app.GameRun {
		if root == other {
			return &app.GameRun{PID: 9, Started: e.Now()}
		}
		return nil
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var tr view.LiveTracker
	l, err := tr.Update(e.App, st)
	if err != nil {
		t.Fatal(err)
	}
	if l.Running || l.Blocked != view.Tf("a game is already running (%s)", "second") {
		t.Fatalf("blocked: %+v", l)
	}
	if p, _ := view.DryRun(e.App, view.ListLaunch); p.Apply {
		t.Fatalf("launch preview offers to launch: %+v", p)
	}
}
