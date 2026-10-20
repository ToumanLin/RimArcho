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

// 10 §3.4: while the game runs, its log's errors become "this run" items; when it exits the
// tracker stops.
func TestLiveTracker(t *testing.T) {
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
	}
	if !found {
		t.Fatalf("no item for the repeated error: %+v", l.Items)
	}
	running = false
	if l, _ = tr.Update(e.App, st); l.Running || len(l.Items) != 0 {
		t.Fatalf("after exit: %+v", l)
	}
}
