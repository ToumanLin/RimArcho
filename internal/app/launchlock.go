package app

import (
	"fmt"
	"time"
)

// lockLaunch guards list reads that sync ModsConfig, list writes and the credit handoff.
// The page's reads and edits overlap (each App session takes its own OS lock), so it waits
// briefly for another holder instead of failing a read or an edit at once; launching keeps
// its own longer wait with a notice (waitLaunch).
func (a *App) lockLaunch() (func(), error) {
	limit := a.launchWaitLimit
	if limit == 0 || limit > 5*time.Second {
		limit = 5 * time.Second
	}
	deadline := time.Now().Add(limit)
	for {
		unlock, err := a.tryLaunch()
		if err == nil || !time.Now().Before(deadline) {
			return unlock, err
		}
		time.Sleep(min(20*time.Millisecond, time.Until(deadline)))
	}
}

// tryLaunch takes the lock if free. App sessions have one caller at a time. Nested list
// operations reuse the lock; separate sessions and processes always acquire their own OS lock.
func (a *App) tryLaunch() (func(), error) {
	if a.launchDepth == 0 {
		unlock, err := lockGameLinks(realOrAbs(a.Home))
		if err != nil {
			return nil, fmt.Errorf("another launch or credit replacement is running: %w", err)
		}
		a.launchUnlock = unlock
	}
	a.launchDepth++
	return func() {
		a.launchDepth--
		if a.launchDepth == 0 {
			a.launchUnlock()
			a.launchUnlock = nil
		}
	}, nil
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
