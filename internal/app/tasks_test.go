package app

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func waitEnded(t *testing.T, ts *Tasks, id string) Task {
	t.Helper()
	for i := 0; i < 4000; i++ {
		for _, x := range ts.List() {
			if x.ID == id && x.Ended() {
				return x
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("task %s did not end", id)
	return Task{}
}

func items(refs ...string) []TaskItem {
	var out []TaskItem
	for _, r := range refs {
		out = append(out, TaskItem{Ref: r, Name: "mod " + r})
	}
	return out
}

// One failure does not stop the task; every item keeps its result (16 §2.16).
func TestTasksItemsGoOnAfterAFailure(t *testing.T) {
	t.Parallel()
	ts := OpenTasks(filepath.Join(t.TempDir(), "tasks.json"))
	ts.Do = func(kind, ref string) error {
		if ref == "b" {
			return errors.New("in use")
		}
		return nil
	}
	sub := ts.Submit("delete", LaneFiles, "3 mods", items("a", "b", "c"))
	got := waitEnded(t, ts, sub.ID)
	if got.State != TaskPartial || got.Done != 3 {
		t.Fatalf("state %s done %d, want part 3", got.State, got.Done)
	}
	if got.Items[1].State != ItemFail || got.Items[1].Why != "in use" || got.Items[2].State != ItemOK {
		t.Fatalf("items %+v", got.Items)
	}
	again, err := ts.Again(sub.ID, ItemFail, "", "1 mod")
	if err != nil || len(again.Items) != 1 || again.Items[0].Ref != "b" {
		t.Fatalf("again %+v %v", again, err)
	}
	if g := waitEnded(t, ts, again.ID); g.State != TaskFailed {
		t.Fatalf("retry state %s", g.State)
	}
	// The failed item belongs to the retry now: a second press finds nothing (VM run B1,
	// where continue the rest pressed twice ran ten deletions twice).
	if _, err := ts.Again(sub.ID, ItemFail, "", "1 mod"); err == nil {
		t.Fatal("the same items were taken again")
	}
	for _, x := range ts.List() {
		if x.ID == sub.ID && x.Items[1].Again != again.ID {
			t.Fatalf("handed-over item %+v", x.Items[1])
		}
	}
}

// Stop: the item in hand finishes, the rest are left untouched; a queued task never runs.
func TestTasksStop(t *testing.T) {
	t.Parallel()
	ts := OpenTasks(filepath.Join(t.TempDir(), "tasks.json"))
	hold := make(chan struct{})
	var once sync.Once
	ts.Do = func(kind, ref string) error {
		if ref == "a" {
			<-hold
		}
		return nil
	}
	first := ts.Submit("delete", LaneFiles, "3 mods", items("a", "b", "c"))
	second := ts.Submit("move", LaneFiles, "1 mod", items("d"))
	for i := 0; i < 4000; i++ {
		if ts.List()[1].State == TaskRunning {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := ts.Stop(second.ID); err != nil {
		t.Fatal(err)
	}
	if err := ts.Stop(first.ID); err != nil {
		t.Fatal(err)
	}
	once.Do(func() { close(hold) })
	got := waitEnded(t, ts, first.ID)
	if got.State != TaskStopped || got.Items[0].State != ItemOK || got.Items[1].State != ItemSkip {
		t.Fatalf("stopped %+v", got)
	}
	if g := waitEnded(t, ts, second.ID); g.State != TaskStopped || g.Start != 0 {
		t.Fatalf("the queued task ran: %+v", g)
	}
}

// The record survives the window; what ran when it closed is marked stopped.
func TestTasksRecordAndClose(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tasks.json")
	ts := OpenTasks(path)
	id := ts.Begin("scan", "main", nil)
	if err := ts.Stop(id); !errors.Is(err, ErrCannotStop) {
		t.Fatalf("stop a scan: %v", err)
	}
	done := ts.Begin("analyze", "main", nil)
	ts.Finish(done, TaskDone, "2 problems")
	ts.Close()
	again := OpenTasks(path).List()
	if len(again) != 2 || again[1].State != TaskStopped || !again[1].Closed || again[0].Result != "2 problems" {
		t.Fatalf("record %+v", again)
	}
	ts2 := OpenTasks(path)
	if n := ts2.ClearDone(); n != 1 || len(ts2.List()) != 1 {
		t.Fatalf("clear done %d", n)
	}
	if n := ts2.Unforget(); n != 1 || len(ts2.List()) != 2 {
		t.Fatalf("unforget %d", n)
	}
}

// Waiting tasks move in their lane's order.
func TestTasksMove(t *testing.T) {
	// Not parallel: waits a fixed time for background work, which is too short under load.
	ts := OpenTasks("")
	block := make(chan struct{})
	ts.Do = func(kind, ref string) error { <-block; return nil }
	ts.Submit("delete", LaneFiles, "a", items("a"))
	b := ts.Submit("delete", LaneFiles, "b", items("b"))
	c := ts.Submit("delete", LaneFiles, "c", items("c"))
	time.Sleep(20 * time.Millisecond)
	if err := ts.Move(c.ID, -1); err != nil {
		t.Fatal(err)
	}
	ts.mu.Lock()
	q := ts.queuedLocked(LaneFiles)
	ts.mu.Unlock()
	if len(q) != 2 || q[0].ID != c.ID || q[1].ID != b.ID {
		t.Fatalf("order %v %v", q[0].Obj, q[1].Obj)
	}
	close(block)
}

func TestStopQueuedWorkshopContinuationKeepsEarlierResults(t *testing.T) {
	ts := OpenTasks("")
	ts.mu.Lock()
	ts.list = []*Task{{ID: "continued", Kind: "subscribe", Lane: LaneSelf, State: TaskQueued, Frac: .5, Done: 2, Total: 3,
		Args:  []string{"subscribe", "workshop", "1", "2", "3"},
		Items: []TaskItem{{Ref: "ws:1", State: ItemOK}, {Ref: "ws:2", State: ItemFail, Permanent: true}, {Ref: "ws:3", State: ItemWait}},
	}}
	ts.mu.Unlock()
	if err := ts.Stop("continued"); err != nil {
		t.Fatal(err)
	}
	got := ts.List()[0]
	if got.State != TaskStopped || got.Items[0].State != ItemOK || got.Items[1].State != ItemFail || got.Items[2].State != ItemSkip || got.Frac != .5 {
		t.Fatalf("cancel discarded earlier results: %+v", got)
	}
}
