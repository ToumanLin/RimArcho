package app

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Maintenance kinds serialize independently, and keep successes and failures (06 §2.16).
func TestSelfTasksQueueResultsAndConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	ts := OpenTasks(path)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer ts.Close()
	defer once.Do(func() { close(release) })
	first := ts.EnqueueSelf(Task{Kind: "db-backup", Obj: "database"}, func(id string) (string, error) {
		ts.Update(id, func(t *Task) { t.Phase, t.Frac = "copying", 0.5 })
		close(started)
		<-release
		return "backup written", nil
	})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first task did not start")
	}
	second := ts.EnqueueSelf(Task{Kind: "db-backup", Obj: "database"}, func(string) (string, error) {
		return "", errors.New("disk full")
	})
	if second.State != TaskQueued || second.Start != 0 {
		t.Fatalf("submission %+v", second)
	}
	for _, task := range ts.List() {
		if task.ID == second.ID && task.State != TaskQueued {
			t.Fatalf("same kind did not wait: %+v", task)
		}
		if task.ID == first.ID && (task.Phase != "copying" || task.Frac != 0.5) {
			t.Fatalf("progress %+v", task)
		}
	}
	other := ts.EnqueueSelf(Task{Kind: "cache-clear", Obj: "previews"}, func(string) (string, error) { return "cache cleared", nil })
	if task := waitEnded(t, ts, other.ID); task.State != TaskDone || task.Result != "cache cleared" {
		t.Fatalf("different kind did not run concurrently: %+v", task)
	}
	once.Do(func() { close(release) })
	if task := waitEnded(t, ts, first.ID); task.State != TaskDone || task.Result != "backup written" || task.Start == 0 || task.End == 0 {
		t.Fatalf("completion %+v", task)
	}
	if task := waitEnded(t, ts, second.ID); task.State != TaskFailed || task.Result != "disk full" {
		t.Fatalf("failure %+v", task)
	}
	for _, task := range OpenTasks(path).List() {
		if task.Result == "" || !task.Ended() || task.Lane != LaneSelf {
			t.Fatalf("persisted result %+v", task)
		}
	}
}

func TestSelfTasksCancelAndClose(t *testing.T) {
	ts := OpenTasks("")
	started, release := make(chan struct{}), make(chan struct{})
	first := ts.EnqueueSelf(Task{Kind: "data-update", Obj: "community"}, func(string) (string, error) {
		close(started)
		<-release
		return "updated", nil
	})
	defer close(release)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not start")
	}
	never := func(string) (string, error) { t.Error("a cancelled task ran"); return "", nil }
	second := ts.EnqueueSelf(Task{Kind: "data-update", Obj: "community"}, never)
	if err := ts.Stop(second.ID); err != nil {
		t.Fatal(err)
	}
	third := ts.EnqueueSelf(Task{Kind: "data-update", Obj: "community"}, never)
	if err := ts.Stop(first.ID); !errors.Is(err, ErrCannotStop) {
		t.Fatalf("running maintenance should finish: %v", err)
	}
	ts.Close()
	for _, task := range ts.List() {
		if task.State != TaskStopped || (task.ID == third.ID && !task.Closed) {
			t.Fatalf("closed %+v", task)
		}
	}
}
