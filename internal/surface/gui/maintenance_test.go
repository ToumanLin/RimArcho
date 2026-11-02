package gui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestMaintenanceWaitsForGUIIdle(t *testing.T) {
	for _, kind := range []string{"game", "scan", "analyze", "task", "page-lock", "closed", "load-cost", "texture-read", "files"} {
		t.Run(kind, func(t *testing.T) {
			e, api, _ := fixture(t)
			switch kind {
			case "game":
				e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 1} }
			case "scan", "analyze":
				api.busy[kind] = true
			case "task":
				id := api.tasks.Begin("test", "test", nil)
				defer api.tasks.Finish(id, app.TaskDone, "")
			case "page-lock":
				api.mu.Lock()
				defer api.mu.Unlock()
			case "closed":
				api.needsClosed = true
			case "load-cost":
				api.costMeasure.page.Loading = true
			case "texture-read":
				api.texMeasure.page = &view.TexturePage{Loading: true}
			case "files":
				api.filesMu.Lock()
				defer api.filesMu.Unlock()
			}
			start := time.Now()
			api.maintenanceCycle(context.Background())
			if time.Since(start) > time.Second {
				t.Fatal("idle check waited on foreground")
			}
			if e.App.Store.Meta(store.MaintenanceLastKey) != "" {
				t.Fatal("ran while not idle")
			}
		})
	}
}

func TestMaintenanceYieldsAndPageDoesNotWait(t *testing.T) {
	e, api, _ := fixture(t)
	testkit.MaintenanceHistory(t, e.App.Store, "old", 20, 1000, 220)
	entered, release := make(chan struct{}), make(chan struct{})
	e.App.FreeSpace = func(string) int64 { close(entered); <-release; return 1 << 40 }
	done := make(chan struct{})
	go func() { api.maintenanceCycle(context.Background()); close(done) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("maintenance did not reach vacuum")
	}
	// A page blocked by maintenance would wait until release, which comes only after
	// this read: the bound catches that without timing a cold read on a slow runner.
	board := make(chan string, 1)
	go func() { board <- api.Board().Error }()
	select {
	case msg := <-board:
		if msg != "" {
			t.Fatal(msg)
		}
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("maintenance blocked page")
	}
	id := api.tasks.Begin("test", "foreground", nil)
	defer api.tasks.Finish(id, app.TaskDone, "")
	// The worker is paused before VACUUM while the idle monitor cancels it.
	time.Sleep(250 * time.Millisecond)
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("did not yield")
	}
	if e.App.Store.Meta(store.MaintenanceLastKey) != "" {
		t.Fatal("cancelled pass marked complete")
	}
}

func TestManualCompactUsesMaintenanceAndIgnoresPeriodAndTasks(t *testing.T) {
	e, api, _ := fixture(t)
	ids := testkit.MaintenanceHistory(t, e.App.Store, "manual", 8, 20, 30)
	if _, err := api.runSettingTask(e.App, "", view.TaskVacuum, []string{"db", "vacuum", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if f, _ := e.App.Store.SnapshotFiles(ids[0]); f == nil {
		t.Fatal("GUI dry run deleted data")
	}
	e.App.Store.SetMeta(store.MaintenanceLastKey, e.Now().Format(time.RFC3339))
	id := api.tasks.Begin("test", "other work", nil)
	defer api.tasks.Finish(id, app.TaskDone, "")
	text, err := api.runSettingTask(e.App, "", view.TaskVacuum, []string{"db", "vacuum"})
	if err != nil || !strings.Contains(text, "freed") || !strings.Contains(text, "(now ") {
		t.Fatalf("manual: %s %v", text, err)
	}
	if f, _ := e.App.Store.SnapshotFiles(ids[0]); f != nil {
		t.Fatal("manual only vacuumed")
	}
}
