package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// A long holder of the launch lock (a test run) writes what it is doing to busy.json in the
// data directory, so a call the lock refuses names the task, when it started and when it
// should end, instead of only "another launch is running" (14 §1 "Busy").

const busyFile = "busy.json"

// LockHolder is the busy.json record of the process that holds the launch lock.
type LockHolder struct {
	Task    string    `json:"task"` // such as "a test run of plan:Colony (Game Config 1.6)"
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
	Until   time.Time `json:"until,omitzero"` // the latest it should end (a test run's time limit)
}

// markBusy records this process as the lock holder until the returned clear runs. Errors
// are ignored: the record only improves another call's message.
func (a *App) markBusy(task string, until time.Time) func() {
	if a.Home == "" {
		return func() {}
	}
	path := filepath.Join(a.Home, busyFile)
	h := LockHolder{Task: task, PID: os.Getpid(), Started: time.Now(), Until: until}
	if b, err := json.Marshal(h); err != nil || os.WriteFile(path, b, 0o644) != nil {
		return func() {}
	}
	return func() {
		if cur, ok := readBusy(a.Home); ok && cur.PID == h.PID && cur.Started.Equal(h.Started) {
			_ = os.Remove(path)
		}
	}
}

func readBusy(home string) (LockHolder, bool) {
	var h LockHolder
	b, err := os.ReadFile(filepath.Join(home, busyFile))
	if err != nil || json.Unmarshal(b, &h) != nil || h.Task == "" {
		return h, false
	}
	return h, true
}

// ErrBusy is the error of a call the launch lock refuses.
type ErrBusy struct {
	Holder *LockHolder // nil when the holder left no record (a short edit or a launch)
	cause  error
}

func (e *ErrBusy) Error() string {
	h := e.Holder
	if h == nil {
		return "busy: another launch, list edit or credit replacement is running in another RimArcho process; try again in a moment"
	}
	s := fmt.Sprintf("busy: %s (process %d) has been running since %s", h.Task, h.PID, h.Started.Local().Format("15:04:05"))
	if !h.Until.IsZero() {
		s += fmt.Sprintf(" and ends by %s at the latest", h.Until.Local().Format("15:04:05"))
	}
	return s + "; this call needs the launch lock it holds, so try again then"
}

func (e *ErrBusy) Unwrap() error { return e.cause }

// busyError wraps the lock's refusal with the holder's record in home. A record whose
// time limit passed over a minute ago is stale (a killed process) and ignored.
func busyError(home string, err error) error {
	var busy *ErrBusy
	if errors.As(err, &busy) {
		return err
	}
	e := &ErrBusy{cause: err}
	if h, ok := readBusy(home); ok && (h.Until.IsZero() || time.Since(h.Until) < time.Minute) {
		e.Holder = &h
	}
	return e
}

// SnapshotReads makes the App's loadout reads use the stored lists, as the window's pages do
// (WorkspaceSnapshot): no launch lock unless ModsConfig.xml changed outside RimArcho and must
// be imported. MCP's read-only tools run under it, so a test run in another process does not
// block them (14 §1). It returns the restore.
func (a *App) SnapshotReads() func() {
	prev := a.workspaceSnapshot
	a.workspaceSnapshot = true
	return func() { a.workspaceSnapshot = prev }
}
