package app

import (
	"fmt"
	"time"
)

const workspaceLockWait = 5 * time.Second

// lockLaunch guards list reads that sync ModsConfig, list writes and the credit handoff.
// The page's reads and edits overlap (each App session takes its own OS lock), so it waits
// briefly for another holder instead of failing a read or an edit at once; launching keeps
// its own longer wait with a notice (waitLaunch).
func (a *App) lockLaunch() (func(), error) {
	if a.launchDepth != 0 {
		return a.tryLaunch()
	}
	limit := a.launchWaitLimit
	if limit == 0 || limit > workspaceLockWait {
		limit = workspaceLockWait
	}
	return pollWorkspaceLock(limit, a.tryLaunch, nil)
}

func pollWorkspaceLock(limit time.Duration, try func() (func(), error), stop func() error) (func(), error) {
	deadline := time.Now().Add(limit)
	for {
		if stop != nil {
			if err := stop(); err != nil {
				return nil, err
			}
		}
		unlock, err := try()
		if err == nil || !time.Now().Before(deadline) {
			return unlock, err
		}
		time.Sleep(min(20*time.Millisecond, time.Until(deadline)))
	}
}

// TryWorkspaceSync limits implicit ModsConfig syncing on a UI's shared session
// to a single attempt. Explicit edits can acquire the guard before using it.
func (a *App) TryWorkspaceSync() { a.tryWorkspaceSync = true }

// WorkspaceLock is an acquired OS lock for a single data directory. Acquire,
// bind and release it on the same goroutine because Windows tracks thread ownership.
type WorkspaceLock struct{ unlock func() }

// LockWorkspace waits without accessing the App that will perform the edit.
func LockWorkspace(home string, stop func() error) (*WorkspaceLock, error) {
	path := realOrAbs(home)
	unlock, err := pollWorkspaceLock(workspaceLockWait, func() (func(), error) {
		return tryWorkspaceLock(path)
	}, stop)
	if err != nil {
		return nil, err
	}
	return &WorkspaceLock{unlock}, nil
}

// Release releases a guard that has not been bound.
func (w *WorkspaceLock) Release() { w.unlock() }

// Bind lends the acquired lock to an idle session for nested operations.
// Its returned release must run before the session is made available again.
func (w *WorkspaceLock) Bind(a *App) func() {
	a.launchDepth, a.launchUnlock = 1, w.Release
	return a.releaseLaunch
}

func tryWorkspaceLock(path string) (func(), error) {
	unlock, err := lockGameLinks(path)
	if err != nil {
		return nil, busyError(path, fmt.Errorf("another launch or credit replacement is running: %w", err))
	}
	return unlock, nil
}

// tryLaunch takes the lock if free. App sessions have one caller at a time. Nested list
// operations reuse the lock; separate sessions and processes always acquire their own OS lock.
func (a *App) tryLaunch() (func(), error) {
	if a.launchDepth == 0 {
		unlock, err := tryWorkspaceLock(realOrAbs(a.Home))
		if err != nil {
			return nil, err
		}
		a.launchUnlock = unlock
	}
	a.launchDepth++
	return a.releaseLaunch, nil
}

func (a *App) releaseLaunch() {
	a.launchDepth--
	if a.launchDepth == 0 {
		a.launchUnlock()
		a.launchUnlock = nil
	}
}

func (a *App) waitLaunch() (func(), error) {
	limit := a.launchWaitLimit
	if limit == 0 {
		limit = 10 * time.Second
	}
	deadline := time.Now().Add(limit)
	notified := false
	for {
		unlock, err := a.tryLaunch()
		if err == nil {
			return unlock, nil
		}
		if !notified {
			if a.LaunchWait != nil {
				a.LaunchWait("waiting for background replacement to finish")
			}
			notified = true
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("waiting for background replacement timed out; try launching again: %w", err)
		}
		time.Sleep(min(50*time.Millisecond, time.Until(deadline)))
	}
}
