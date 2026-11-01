package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// The record file is shared by every window open on the same data folder (the GUI, the
// TUI, a second GUI; 06 §2.16). Each window runs and answers for the tasks it started
// (Task.Owner); another window's tasks come from the file: the ended ones are shown, the
// unfinished ones are not (their window has them in memory and writes them when they
// end). A write reads the file back first, keeps the other windows' records and drops
// the ones this window took out, under a lock across processes.

// taskSeq numbers task IDs within the process; the process ID keeps two windows apart.
var taskSeq atomic.Int64

// openOwners are this process's open records (Tasks not yet closed), by owner.
var (
	ownersMu   sync.Mutex
	openOwners = map[string]bool{}
	selfStart  = sync.OnceValue(func() int64 { s, _ := processStart(uint32(os.Getpid())); return s })
)

// newTaskOwner names one open record: process, its start time (so a reused process ID is
// not taken for the window that ended) and a number for several records in one process.
func newTaskOwner() string {
	o := fmt.Sprintf("%d.%d.%d", os.Getpid(), selfStart(), taskSeq.Add(1))
	ownersMu.Lock()
	openOwners[o] = true
	ownersMu.Unlock()
	return o
}

func closeTaskOwner(o string) {
	ownersMu.Lock()
	delete(openOwners, o)
	ownersMu.Unlock()
}

// ownerAlive reports whether the window that started a task is still open.
func ownerAlive(owner string) bool {
	if owner == "" {
		return false // written before records had owners
	}
	ownersMu.Lock()
	open := openOwners[owner]
	ownersMu.Unlock()
	if open {
		return true
	}
	var pid uint32
	var start, n int64
	if _, err := fmt.Sscanf(owner, "%d.%d.%d", &pid, &start, &n); err != nil {
		return false
	}
	if int(pid) == os.Getpid() && start == selfStart() {
		return false // this process, a record it closed
	}
	s, alive := processStart(pid)
	return alive && (s == 0 || start == 0 || s == start)
}

// readTaskFile reads the record; ok is false when the file is there but cannot be read
// (then nothing in it is taken as removed).
func readTaskFile(path string) (list []*Task, ok bool) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, true
	}
	if err != nil || json.Unmarshal(b, &list) != nil {
		return nil, false
	}
	return list, true
}

type fileStamp struct {
	mod  time.Time
	size int64
}

func stampOf(path string) fileStamp {
	fi, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{fi.ModTime(), fi.Size()}
}

// carryMarks copies what only goes one way (a failure looked at, an item taken over by
// another task) from src to dst, so marks made in either window survive the other's write.
func carryMarks(dst, src *Task) {
	dst.Seen = dst.Seen || src.Seen
	if len(dst.Items) != len(src.Items) {
		return
	}
	for i := range dst.Items {
		if a := src.Items[i].Again; a != "" && (dst.Items[i].Again == "" || dst.Items[i].Again == "pending") {
			dst.Items[i].Again = a
		}
	}
}

// trimTasks keeps everything running or waiting and the newest TaskLimit that ended (the
// done ones go first when there are too many).
func trimTasks(list []*Task) []*Task {
	ended := 0
	for _, t := range list {
		if t.Ended() {
			ended++
		}
	}
	over := ended - TaskLimit
	if over <= 0 {
		return list
	}
	keep := append([]*Task(nil), list...)
	for _, drop := range []func(*Task) bool{
		func(t *Task) bool { return t.State == TaskDone },
		func(t *Task) bool { return t.Ended() },
	} {
		for i := len(keep) - 1; i >= 0 && over > 0; i-- {
			if drop(keep[i]) {
				keep = append(keep[:i], keep[i+1:]...)
				over--
			}
		}
	}
	return keep
}

func newestFirst(list []*Task) {
	sort.SliceStable(list, func(a, b int) bool { return list[a].Queued > list[b].Queued })
}

// syncLocked merges the file with this window's record, and writes the result when write
// is set. This window's tasks are as it has them, except ended ones another window took
// out of the file or took over; the other windows' are as the file has them, less the
// ones this window took out.
func (ts *Tasks) syncLocked(write bool) {
	unlock := lockTaskFile(ts.path)
	defer unlock()
	disk, ok := readTaskFile(ts.path)
	onDisk := make(map[string]*Task, len(disk))
	for _, d := range disk {
		onDisk[d.ID] = d
	}
	inMem := make(map[string]*Task, len(ts.list))
	for _, t := range ts.list {
		inMem[t.ID] = t
	}
	var file, mem []*Task
	own := map[string]bool{}
	for _, t := range ts.list {
		if t.Owner != ts.owner {
			continue
		}
		d := onDisk[t.ID]
		if ok && t.Ended() && ts.written[t.ID] {
			if d == nil {
				continue // taken out in another window
			}
			if d.Owner != t.Owner {
				continue // another window took it over since (continue the rest, put back)
			}
		}
		if d != nil {
			carryMarks(t, d)
		}
		own[t.ID] = true
		file = append(file, t)
		mem = append(mem, t)
	}
	if !ok {
		// The file cannot be read: keep the other windows' records as last read.
		for _, t := range ts.list {
			if !own[t.ID] && !ts.gone[t.ID] {
				file = append(file, t)
				mem = append(mem, t)
			}
		}
	}
	for _, d := range disk {
		if own[d.ID] || d.Owner == ts.owner || ts.gone[d.ID] {
			continue
		}
		if m := inMem[d.ID]; m != nil {
			carryMarks(d, m)
		}
		file = append(file, d)
		if !d.Ended() {
			continue // runs in another window
		}
		if m := inMem[d.ID]; m != nil {
			*m = *d // keep the pointer: callers may hold it
			d = m
		}
		mem = append(mem, d)
	}
	newestFirst(file)
	newestFirst(mem)
	if write {
		file = trimTasks(file)
		kept := make(map[string]bool, len(file))
		ts.written = map[string]bool{}
		for _, t := range file {
			kept[t.ID] = true
			if t.Owner == ts.owner {
				ts.written[t.ID] = true
			}
		}
		mem = slicesFilter(mem, func(t *Task) bool { return kept[t.ID] || !t.Ended() })
		if b, err := json.Marshal(file); err == nil {
			os.MkdirAll(filepath.Dir(ts.path), 0o755)
			tmp := ts.path + ".tmp"
			if os.WriteFile(tmp, b, 0o644) == nil {
				os.Rename(tmp, ts.path)
			}
		}
		clear(ts.gone)
	}
	ts.list = mem
	ts.stampSeen = stampOf(ts.path)
}

func slicesFilter(list []*Task, keep func(*Task) bool) []*Task {
	out := list[:0]
	for _, t := range list {
		if keep(t) {
			out = append(out, t)
		}
	}
	return out
}

// reloadLocked takes in what other windows wrote since this one last read the file. A
// file written in the last two seconds is read again even when it looks the same: two
// writes within the clock's tick can leave the same time and size.
func (ts *Tasks) reloadLocked() {
	if ts.path == "" {
		return
	}
	if s := stampOf(ts.path); s == ts.stampSeen && time.Since(s.mod) > 2*time.Second {
		return
	}
	ts.syncLocked(false)
}
