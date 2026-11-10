package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Two windows on one data folder (16 §2.16): each keeps the other's records when it
// writes, and sees what the other ended.
func TestTasksTwoWindowsKeepEachOthersRecords(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tasks.json")
	gui, tui := OpenTasks(path), OpenTasks(path)
	defer gui.Close()
	defer tui.Close()
	a := gui.Begin("analyze", "main", nil)
	gui.Finish(a, TaskDone, "2 problems")
	b := tui.Begin("subscribe", "git", nil)
	tui.Finish(b, TaskDone, "subscribed")
	if got := onFile(t, path); !slices.Contains(got, a) || !slices.Contains(got, b) {
		t.Fatalf("file %v, want %s and %s", got, a, b)
	}
	if got := taskIDs(gui.List()); !slices.Contains(got, b) {
		t.Fatalf("the window does not see the TUI's task: %v", got)
	}
	c := gui.Begin("scan", "main", nil)
	gui.Finish(c, TaskDone, "")
	if got := onFile(t, path); len(got) != 3 {
		t.Fatalf("after another write: %v", got)
	}
}

// Opening a window does not stop what another open window runs, and does not show it
// until it ends; a task whose window ended without closing is stopped and taken over.
func TestTasksOtherWindowsRunningTasks(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tasks.json")
	gui := OpenTasks(path)
	defer gui.Close()
	running := gui.Begin("scan", "main", nil)
	ended := gui.Begin("analyze", "main", nil)
	gui.Finish(ended, TaskDone, "") // writes the running one too
	tui := OpenTasks(path)
	defer tui.Close()
	if got := taskIDs(tui.List()); slices.Contains(got, running) || !slices.Contains(got, ended) {
		t.Fatalf("the TUI shows %v", got)
	}
	for _, x := range readBack(t, path) {
		if x.ID == running && x.State != TaskRunning {
			t.Fatalf("the window's scan was stopped by opening the TUI: %+v", x)
		}
	}
	gui.Finish(running, TaskDone, "1 299 mods")
	if x := find(tui.List(), running); x == nil || x.Result != "1 299 mods" {
		t.Fatalf("the ended scan in the TUI: %+v", x)
	}

	// A window that ended without closing: its unfinished task is stopped by the next one.
	gone := OpenTasks(path)
	left := gone.Begin("pack", "colony", nil)
	gone.Finish(gone.Begin("cache-clear", "previews", nil), TaskDone, "")
	closeTaskOwner(gone.owner)
	next := OpenTasks(path)
	defer next.Close()
	if x := find(next.List(), left); x == nil || x.State != TaskStopped || !x.Closed || x.Owner != next.owner {
		t.Fatalf("left behind: %+v", x)
	}
	for _, x := range readBack(t, path) {
		if x.ID == left && x.State != TaskStopped {
			t.Fatalf("on file: %+v", x)
		}
	}
}

// Taking a record out in one window takes it out of the file and the other window; the
// owner does not write it back. Putting it back brings it to both. A failure looked at
// in one window stays looked at.
func TestTasksForgetAndSeenAcrossWindows(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tasks.json")
	gui, tui := OpenTasks(path), OpenTasks(path)
	defer gui.Close()
	defer tui.Close()
	done := gui.Begin("scan", "main", nil)
	gui.Finish(done, TaskDone, "")
	failed := gui.Begin("analyze", "main", nil)
	gui.Finish(failed, TaskFailed, "database locked")
	tui.List()
	if n := tui.Forget([]string{done}); n != 1 {
		t.Fatalf("forget %d", n)
	}
	tui.MarkSeen()
	gui.Finish(gui.Begin("cache-clear", "previews", nil), TaskDone, "")
	if got := onFile(t, path); slices.Contains(got, done) {
		t.Fatalf("the owner wrote the forgotten task back: %v", got)
	}
	if find(gui.List(), done) != nil {
		t.Fatal("the window still lists the task the TUI took out")
	}
	if x := findOnFile(readBack(t, path), failed); x == nil || !x.Seen {
		t.Fatalf("looked-at failure: %+v", x)
	}
	if n := tui.Unforget(); n != 1 {
		t.Fatalf("unforget %d", n)
	}
	gui.Finish(gui.Begin("scan", "main", nil), TaskDone, "")
	if find(gui.List(), done) == nil || !slices.Contains(onFile(t, path), done) {
		t.Fatalf("put back: file %v", onFile(t, path))
	}
}

// A record written before tasks had owners is read as before.
func TestTasksRecordWithoutOwners(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tasks.json")
	os.WriteFile(path, []byte(`[{"id":"t1-1","kind":"scan","lane":"self","state":"run","queued":2},{"id":"t0-1","kind":"analyze","lane":"self","state":"done","queued":1}]`), 0o644)
	ts := OpenTasks(path)
	defer ts.Close()
	got := ts.List()
	if len(got) != 2 || got[0].State != TaskStopped || !got[0].Closed || got[1].State != TaskDone {
		t.Fatalf("old record %+v", got)
	}
	if fmt.Sprint(onFile(t, path)) != "[t1-1 t0-1]" {
		t.Fatalf("file %v", onFile(t, path))
	}
}

func readBack(t *testing.T, path string) []*Task {
	t.Helper()
	list, ok := readTaskFile(path)
	if !ok {
		t.Fatal("unreadable record")
	}
	return list
}

func onFile(t *testing.T, path string) []string {
	t.Helper()
	var out []string
	for _, x := range readBack(t, path) {
		out = append(out, x.ID)
	}
	return out
}

func taskIDs(list []Task) []string {
	var out []string
	for _, x := range list {
		out = append(out, x.ID)
	}
	return out
}

func find(list []Task, id string) *Task {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

func findOnFile(list []*Task, id string) *Task {
	for _, x := range list {
		if x.ID == id {
			return x
		}
	}
	return nil
}

// A write that fails (disk full, a scanner holding the file) does not count as written:
// the finished task stays listed, and lands in the file once a write gets through.
func TestTasksFailedWriteKeepsFinishedTasks(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tasks.json")
	ts := OpenTasks(path)
	defer ts.Close()
	ts.Finish(ts.Begin("scan", "main", nil), TaskDone, "")
	block := path + ".tmp"
	if err := os.Mkdir(block, 0o755); err != nil {
		t.Fatal(err)
	}
	b := ts.Begin("analyze", "main", nil)
	ts.Finish(b, TaskDone, "2 problems")
	ts.Finish(ts.Begin("cache-clear", "previews", nil), TaskDone, "")
	if find(ts.List(), b) == nil {
		t.Fatal("a failed write dropped the finished task")
	}
	os.Remove(block)
	ts.Finish(ts.Begin("scan", "main", nil), TaskDone, "")
	if !slices.Contains(onFile(t, path), b) {
		t.Fatalf("file %v, want %s", onFile(t, path), b)
	}
}
