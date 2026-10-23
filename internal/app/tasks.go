package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"
)

// Background tasks (06 §2.16): what RimArcho does after a click that takes a while, kept
// as one record per task so the window can say what runs, what waits and what each item
// came to. Two lanes run here, one task at a time, one item at a time: "files" (Mod 库's
// actions on folders) and "steam" (退订 and 重新订阅 through the Steam client). The third
// lane, "self", is RimArcho's own jobs (scan, analysis, packs, textures, downloads); they
// keep their own runners and only report into the record (Begin, Update, Finish).
//
// The record survives the window in <data>/tasks.json; what still runs when the window
// closes is marked stopped (00 §4 principle 6: RimArcho does not stay in the background).

// Task lanes.
const (
	LaneFiles = "files"
	LaneSteam = "steam"
	LaneSelf  = "self"
)

// Task states.
const (
	TaskQueued  = "queue"
	TaskRunning = "run"
	TaskDone    = "done"
	TaskFailed  = "fail"
	TaskPartial = "part" // some items done, some failed
	TaskStopped = "stop"
)

// Item states.
const (
	ItemWait = "wait"
	ItemRun  = "run"
	ItemOK   = "ok"
	ItemFail = "fail"
	ItemSkip = "skip" // left when the task stopped
)

// TaskItem is one thing a task works on (a copy of a mod).
type TaskItem struct {
	Ref   string `json:"ref"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes,omitempty"`
	State string `json:"state"`
	Why   string `json:"why,omitempty"` // a failure's reason
}

// Task is one background task.
type Task struct {
	ID     string     `json:"id"`
	Kind   string     `json:"kind"` // the Mod 库 op (view.Lib*) or scan, analyze, pack, textures, textures-clean, download
	Lane   string     `json:"lane"`
	Obj    string     `json:"obj"` // what it works on: "14 mods", a pack's name
	State  string     `json:"state"`
	Phase  string     `json:"phase,omitempty"`  // what it does now
	Result string     `json:"result,omitempty"` // its end in a sentence (self tasks; the lanes' is counted from the items)
	Closed bool       `json:"closed,omitempty"` // stopped because the window closed
	Queued int64      `json:"queued"`           // unix ms
	Start  int64      `json:"start,omitempty"`
	End    int64      `json:"end,omitempty"`
	Done   int        `json:"done,omitempty"`
	Total  int        `json:"total,omitempty"`
	Frac   float64    `json:"frac,omitempty"` // a self task's progress when it has no count
	Step   int        `json:"step,omitempty"` // a pack build's step (1–5)
	Items  []TaskItem `json:"items,omitempty"`
	Seen   bool       `json:"seen,omitempty"`   // a failure the player has looked at
	Target string     `json:"target,omitempty"` // a pack's name, for its page
}

// Ended reports whether the task has finished one way or another.
func (t *Task) Ended() bool {
	return t.State != TaskQueued && t.State != TaskRunning
}

// Counts are the items by state.
func (t *Task) Counts() map[string]int {
	c := map[string]int{}
	for _, it := range t.Items {
		c[it.State]++
	}
	return c
}

// TaskLimit is how many finished tasks the record keeps.
const TaskLimit = 200

// Tasks is the record and the two lanes' runners. Do runs one item of a lane's task (the
// window passes Mod 库's single-item action); Gate holds a lane while it says false (the
// files lane waits for a scan); Changed is told after every change; Ended after a task
// ends. None is called with the record's lock held.
type Tasks struct {
	Do func(kind, ref string) error
	// Batch, when it takes the kind (true), runs a whole task at once: it may check every
	// item first, work on several together and rescan once at the end. It reports each item
	// through b; an item it never reports was not touched.
	Batch   func(kind string, refs []string, b BatchCalls) bool
	Gate    func(lane string) bool
	Changed func()
	Ended   func(t Task)

	mu       sync.Mutex
	path     string
	list     []*Task
	stopping map[string]bool
	stops    map[string]func() error // a self task's stop
	working  map[string]bool         // lanes with a runner
	seq      int
	now      func() time.Time
	forgot   [][2]any // the last Forget: tasks and their places, for Unforget
}

// BatchCalls are how a Batch reports on its items (by their index in the task).
type BatchCalls struct {
	Stopped func() bool // the player asked the task to stop
	Start   func(i int) // item i is being worked on
	Done    func(i int, err error)
}

// OpenTasks reads the record at path (missing: empty). Tasks that were running or queued
// when the last window closed become stopped.
func OpenTasks(path string) *Tasks {
	ts := &Tasks{path: path, stopping: map[string]bool{}, stops: map[string]func() error{}, working: map[string]bool{}, now: time.Now}
	if b, err := os.ReadFile(path); err == nil {
		var list []*Task
		if json.Unmarshal(b, &list) == nil {
			ts.list = list
		}
	}
	changed := false
	for _, t := range ts.list {
		if !t.Ended() {
			ts.closeLocked(t)
			changed = true
		}
	}
	if changed {
		ts.saveLocked()
	}
	return ts
}

func (ts *Tasks) ms() int64 { return ts.now().UnixMilli() }

// stamp is a queue time later than every one given before, so the order is the order of
// submission even within a millisecond.
func (ts *Tasks) stamp() int64 {
	n := ts.ms()
	for _, t := range ts.list {
		if t.Queued >= n {
			n = t.Queued + 1
		}
	}
	return n
}

func (ts *Tasks) newID() string {
	ts.seq++
	return fmt.Sprintf("t%d-%d", ts.now().UnixNano()/1e6, ts.seq)
}

// List is a copy of the record, newest first.
func (ts *Tasks) List() []Task {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	out := make([]Task, len(ts.list))
	for i, t := range ts.list {
		out[i] = *t
		out[i].Items = slices.Clone(t.Items)
	}
	return out
}

func (ts *Tasks) find(id string) *Task {
	for _, t := range ts.list {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// Submit queues a task of the files or steam lane on items (their Ref and Name; the rest
// is filled in) and starts the lane's runner if it is idle.
func (ts *Tasks) Submit(kind, lane, obj string, items []TaskItem) *Task {
	ts.mu.Lock()
	t := &Task{ID: ts.newID(), Kind: kind, Lane: lane, Obj: obj, State: TaskQueued, Queued: ts.stamp(), Total: len(items)}
	for _, it := range items {
		it.State, it.Why = ItemWait, ""
		t.Items = append(t.Items, it)
	}
	ts.list = append([]*Task{t}, ts.list...)
	start := !ts.working[lane]
	ts.working[lane] = true
	cp := *t
	ts.mu.Unlock()
	ts.changed()
	if start {
		go ts.run(lane)
	}
	return &cp
}

// Begin records a self task that has started; stop (nil: it cannot be stopped) is what x
// does to it.
func (ts *Tasks) Begin(kind, obj string, stop func() error) string {
	ts.mu.Lock()
	t := &Task{ID: ts.newID(), Kind: kind, Lane: LaneSelf, Obj: obj, State: TaskRunning, Queued: ts.stamp(), Start: ts.ms()}
	ts.list = append([]*Task{t}, ts.list...)
	if stop != nil {
		ts.stops[t.ID] = stop
	}
	ts.mu.Unlock()
	ts.changed()
	return t.ID
}

// Update changes a running task (a self task's progress).
func (ts *Tasks) Update(id string, f func(t *Task)) {
	ts.mu.Lock()
	t := ts.find(id)
	if t == nil || t.Ended() {
		ts.mu.Unlock()
		return
	}
	f(t)
	ts.mu.Unlock()
	ts.changed()
}

// Finish ends a self task: done, fail or stop, with its result in a sentence.
func (ts *Tasks) Finish(id, state, result string) {
	ts.mu.Lock()
	t := ts.find(id)
	if t == nil || t.Ended() {
		ts.mu.Unlock()
		return
	}
	t.State, t.Result, t.End, t.Phase = state, result, ts.ms(), ""
	if state == TaskDone {
		t.Frac = 1
		t.Done = t.Total
	}
	delete(ts.stops, id)
	delete(ts.stopping, id)
	ts.saveLocked()
	cp := *t
	ts.mu.Unlock()
	ts.changed()
	if ts.Ended != nil {
		ts.Ended(cp)
	}
}

// ErrCannotStop: the task has no way to stop part way (a scan, an analysis).
var ErrCannotStop = errors.New("it cannot be stopped part way")

// Stop stops a task: a queued one never starts; a running one of a lane stops after the
// item it is on; a self task is asked to stop.
func (ts *Tasks) Stop(id string) error {
	ts.mu.Lock()
	t := ts.find(id)
	if t == nil {
		ts.mu.Unlock()
		return errors.New("no such task")
	}
	switch {
	case t.State == TaskQueued:
		t.State, t.End = TaskStopped, ts.ms()
		for i := range t.Items {
			t.Items[i].State = ItemSkip
		}
		ts.saveLocked()
	case t.State == TaskRunning && t.Lane != LaneSelf:
		ts.stopping[id] = true
	case t.State == TaskRunning:
		stop := ts.stops[id]
		if stop != nil {
			ts.stopping[id] = true
		}
		ts.mu.Unlock()
		if stop == nil {
			return ErrCannotStop
		}
		return stop()
	default:
		ts.mu.Unlock()
		return errors.New("it has ended")
	}
	ts.mu.Unlock()
	ts.changed()
	return nil
}

// StopAsked reports whether the player asked a self task to stop (its runner then ends
// it as stopped rather than failed).
func (ts *Tasks) StopAsked(id string) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.stopping[id]
}

// Again queues a new task with the items of id in state (ItemFail for 重试, ItemSkip for
// 继续剩下的); ref (not "") takes only that item.
func (ts *Tasks) Again(id, state, ref, obj string) (*Task, error) {
	ts.mu.Lock()
	t := ts.find(id)
	if t == nil || t.Lane == LaneSelf {
		ts.mu.Unlock()
		return nil, errors.New("no such task")
	}
	var items []TaskItem
	for _, it := range t.Items {
		if it.State == state && (ref == "" || it.Ref == ref) {
			items = append(items, it)
		}
	}
	kind, lane := t.Kind, t.Lane
	t.Seen = true
	ts.mu.Unlock()
	if len(items) == 0 {
		return nil, errors.New("nothing to do again")
	}
	return ts.Submit(kind, lane, obj, items), nil
}

// Move moves a queued task by d places in its lane's order (-1: sooner).
func (ts *Tasks) Move(id string, d int) error {
	ts.mu.Lock()
	t := ts.find(id)
	if t == nil || t.State != TaskQueued {
		ts.mu.Unlock()
		return errors.New("only a waiting task moves")
	}
	q := ts.queuedLocked(t.Lane)
	i := slices.Index(q, t)
	j := i + d
	if j < 0 || j >= len(q) {
		ts.mu.Unlock()
		return errors.New("it is already there")
	}
	q[i].Queued, q[j].Queued = q[j].Queued, q[i].Queued
	ts.sortLocked()
	ts.mu.Unlock()
	ts.changed()
	return nil
}

// queuedLocked: a lane's waiting tasks, the next one first.
func (ts *Tasks) queuedLocked(lane string) []*Task {
	var q []*Task
	for _, t := range ts.list {
		if t.Lane == lane && t.State == TaskQueued {
			q = append(q, t)
		}
	}
	sort.SliceStable(q, func(a, b int) bool { return q[a].Queued < q[b].Queued })
	return q
}

func (ts *Tasks) sortLocked() {
	sort.SliceStable(ts.list, func(a, b int) bool { return ts.list[a].Queued > ts.list[b].Queued })
}

// Forget takes ended tasks out of the record (their files are not touched); Unforget
// puts the last ones back.
func (ts *Tasks) Forget(ids []string) int {
	ts.mu.Lock()
	var gone [][2]any
	keep := ts.list[:0:0]
	for i, t := range ts.list {
		if slices.Contains(ids, t.ID) && t.Ended() {
			gone = append(gone, [2]any{t, i})
			continue
		}
		keep = append(keep, t)
	}
	ts.list = keep
	if len(gone) > 0 {
		ts.forgot = gone
		ts.saveLocked()
	}
	ts.mu.Unlock()
	ts.changed()
	return len(gone)
}

// ClearDone forgets every task that ended done (failures, partial and stopped ones stay).
func (ts *Tasks) ClearDone() int {
	ts.mu.Lock()
	var ids []string
	for _, t := range ts.list {
		if t.State == TaskDone {
			ids = append(ids, t.ID)
		}
	}
	ts.mu.Unlock()
	return ts.Forget(ids)
}

// Unforget puts the tasks the last Forget took back where they were.
func (ts *Tasks) Unforget() int {
	ts.mu.Lock()
	gone := ts.forgot
	ts.forgot = nil
	for _, g := range gone {
		t, i := g[0].(*Task), g[1].(int)
		if i > len(ts.list) {
			i = len(ts.list)
		}
		ts.list = slices.Insert(ts.list, i, t)
	}
	if len(gone) > 0 {
		ts.saveLocked()
	}
	ts.mu.Unlock()
	ts.changed()
	return len(gone)
}

// MarkSeen marks every ended failure as looked at (the page was opened).
func (ts *Tasks) MarkSeen() {
	ts.mu.Lock()
	n := 0
	for _, t := range ts.list {
		if t.Ended() && !t.Seen {
			t.Seen = true
			n++
		}
	}
	if n > 0 {
		ts.saveLocked()
	}
	ts.mu.Unlock()
	if n > 0 {
		ts.changed()
	}
}

// Close marks what still runs or waits as stopped by the window closing, and saves.
func (ts *Tasks) Close() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for _, t := range ts.list {
		if !t.Ended() {
			ts.stopping[t.ID] = true
			ts.closeLocked(t)
		}
	}
	ts.saveLocked()
}

func (ts *Tasks) closeLocked(t *Task) {
	t.State, t.Closed, t.Phase = TaskStopped, true, ""
	if t.End == 0 {
		t.End = ts.ms()
	}
	for i := range t.Items {
		if t.Items[i].State == ItemWait || t.Items[i].State == ItemRun {
			t.Items[i].State = ItemSkip
		}
	}
}

func (ts *Tasks) changed() {
	if ts.Changed != nil {
		ts.Changed()
	}
}

// run is a lane's runner: the oldest waiting task, item by item, until none waits.
func (ts *Tasks) run(lane string) {
	for {
		ts.mu.Lock()
		q := ts.queuedLocked(lane)
		if len(q) == 0 {
			ts.working[lane] = false
			ts.mu.Unlock()
			return
		}
		t := q[0]
		t.State, t.Start = TaskRunning, ts.ms()
		ts.mu.Unlock()
		ts.changed()
		ts.runTask(t)
	}
}

func (ts *Tasks) runTask(t *Task) {
	if ts.runBatch(t) {
		ts.mu.Lock()
		ts.endLocked(t)
		return
	}
	for i := 0; ; i++ {
		ts.mu.Lock()
		if i >= len(t.Items) || ts.stopping[t.ID] || t.State != TaskRunning {
			break
		}
		ts.mu.Unlock()
		for ts.Gate != nil && !ts.Gate(t.Lane) {
			time.Sleep(300 * time.Millisecond)
			ts.mu.Lock()
			stop := ts.stopping[t.ID] || t.State != TaskRunning
			ts.mu.Unlock()
			if stop {
				break
			}
		}
		ts.mu.Lock()
		if ts.stopping[t.ID] || t.State != TaskRunning {
			break
		}
		t.Items[i].State = ItemRun
		t.Phase = t.Items[i].Name
		ref := t.Items[i].Ref
		ts.mu.Unlock()
		ts.changed()
		var err error
		if ts.Do == nil {
			err = errors.New("nothing runs this task")
		} else {
			err = ts.do(t.Kind, ref)
		}
		ts.mu.Lock()
		if err != nil {
			t.Items[i].State, t.Items[i].Why = ItemFail, err.Error()
		} else {
			t.Items[i].State = ItemOK
		}
		t.Done++
		ts.mu.Unlock()
		ts.changed()
	}
	ts.endLocked(t)
}

// runBatch hands the whole task to Batch when it takes the kind.
func (ts *Tasks) runBatch(t *Task) bool {
	if ts.Batch == nil {
		return false
	}
	stopped := func() bool {
		ts.mu.Lock()
		defer ts.mu.Unlock()
		return ts.stopping[t.ID] || t.State != TaskRunning
	}
	for ts.Gate != nil && !ts.Gate(t.Lane) {
		if stopped() {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	ts.mu.Lock()
	refs := make([]string, len(t.Items))
	for i := range t.Items {
		refs[i] = t.Items[i].Ref
	}
	ts.mu.Unlock()
	b := BatchCalls{Stopped: stopped,
		Start: func(i int) {
			ts.mu.Lock()
			if i >= 0 && i < len(t.Items) && t.Items[i].State == ItemWait {
				t.Items[i].State = ItemRun
				t.Phase = t.Items[i].Name
			}
			ts.mu.Unlock()
			ts.changed()
		},
		Done: func(i int, err error) {
			ts.mu.Lock()
			if i >= 0 && i < len(t.Items) && (t.Items[i].State == ItemWait || t.Items[i].State == ItemRun) {
				if err != nil {
					t.Items[i].State, t.Items[i].Why = ItemFail, err.Error()
				} else {
					t.Items[i].State = ItemOK
				}
				t.Done++
			}
			ts.mu.Unlock()
			ts.changed()
		}}
	return ts.Batch(t.Kind, refs, b)
}

// endLocked ends a lane task after its items (the lock is held; it is released).
func (ts *Tasks) endLocked(t *Task) {
	if t.State == TaskRunning {
		for i := range t.Items {
			if t.Items[i].State == ItemWait || t.Items[i].State == ItemRun {
				t.Items[i].State = ItemSkip
			}
		}
		c := t.Counts()
		switch {
		case ts.stopping[t.ID] && c[ItemSkip] > 0:
			t.State = TaskStopped
		case c[ItemFail] == 0:
			t.State = TaskDone
		case c[ItemOK] == 0:
			t.State = TaskFailed
		default:
			t.State = TaskPartial
		}
		t.End, t.Phase = ts.ms(), ""
	}
	delete(ts.stopping, t.ID)
	ts.saveLocked()
	cp := *t
	cp.Items = slices.Clone(t.Items)
	ts.mu.Unlock()
	ts.changed()
	if ts.Ended != nil {
		ts.Ended(cp)
	}
}

// do runs one item; a panic in it fails the item, not the window.
func (ts *Tasks) do(kind, ref string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return ts.Do(kind, ref)
}

// saveLocked writes the record: everything running or waiting and the newest TaskLimit
// that ended (the done ones go first when there are too many).
func (ts *Tasks) saveLocked() {
	ended := 0
	for _, t := range ts.list {
		if t.Ended() {
			ended++
		}
	}
	if over := ended - TaskLimit; over > 0 {
		for _, drop := range []func(*Task) bool{
			func(t *Task) bool { return t.State == TaskDone },
			func(t *Task) bool { return t.Ended() },
		} {
			for i := len(ts.list) - 1; i >= 0 && over > 0; i-- {
				if drop(ts.list[i]) {
					ts.list = slices.Delete(ts.list, i, i+1)
					over--
				}
			}
		}
	}
	if ts.path == "" {
		return
	}
	b, err := json.Marshal(ts.list)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(ts.path), 0o755)
	tmp := ts.path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, ts.path)
	}
}
