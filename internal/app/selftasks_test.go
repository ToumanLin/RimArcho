package app

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Maintenance kinds serialize independently, and keep successes and failures (16 §2.16).
func TestSelfTasksQueueResultsAndConcurrency(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// 16 §8.9: repeated requests while one runs give one waiting task, not a line of them.
func TestSelfTasksJoinAWaitingTwin(t *testing.T) {
	t.Parallel()
	ts := OpenTasks("")
	defer ts.Close()
	started, release := make(chan struct{}), make(chan struct{})
	var runs sync.WaitGroup
	runs.Add(2)
	count := 0
	var mu sync.Mutex
	run := func(string) (string, error) {
		mu.Lock()
		count++
		first := count == 1
		mu.Unlock()
		if first {
			close(started)
			<-release
		}
		runs.Done()
		return "checked", nil
	}
	req := Task{Kind: "source-check", Obj: "RimArcho", Args: []string{"source-check", "workshop"}}
	first := ts.EnqueueSelf(req, run)
	<-started
	var queued []string
	for i := 0; i < 10; i++ {
		queued = append(queued, ts.EnqueueSelf(req, run).ID)
	}
	for _, id := range queued[1:] {
		if id != queued[0] || id == first.ID {
			t.Fatalf("clicks queued %v after %s", queued, first.ID)
		}
	}
	other := req
	other.Args = []string{"source-check", "git"}
	if ts.EnqueueSelf(other, func(string) (string, error) { return "", nil }).ID == queued[0] {
		t.Fatal("another source joined the workshop check")
	}
	close(release)
	runs.Wait()
	waitEnded(t, ts, queued[0])
	mu.Lock()
	defer mu.Unlock()
	if count != 2 {
		t.Fatalf("11 requests ran %d times, want 2", count)
	}
}

// 16 §8.9: work that repeats by itself keeps one row: a finished copy of the same kind and
// object gives its row to the next run; failures and other objects stay.
func TestBeginOnceKeepsOneRow(t *testing.T) {
	t.Parallel()
	ts := OpenTasks(filepath.Join(t.TempDir(), "tasks.json"))
	defer ts.Close()
	a := ts.BeginOnce("analyze", "Main", nil)
	ts.Finish(a, TaskDone, "analysis done")
	other := ts.BeginOnce("analyze", "Other", nil)
	ts.Finish(other, TaskDone, "analysis done")
	failed := ts.BeginOnce("analyze", "Main", nil)
	ts.Finish(failed, TaskFailed, "no disk")
	last := ts.BeginOnce("analyze", "Main", nil)
	ts.Finish(last, TaskDone, "analysis done")
	var ids []string
	for _, task := range ts.List() {
		ids = append(ids, task.ID)
	}
	want := map[string]bool{last: true, failed: true, other: true}
	if len(ids) != 3 {
		t.Fatalf("rows %v, want the last Main, the failed Main and Other", ids)
	}
	for _, id := range ids {
		if !want[id] {
			t.Fatalf("row %s should have given way (rows %v)", id, ids)
		}
	}
}
