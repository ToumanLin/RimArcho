//go:build !windows

package app

import (
	"os"
	"sync"
	"time"
)

var updateLocks sync.Map

// lockUpdate: outside Windows (not released yet) only installs in this process exclude
// each other; the file marks the folder as in use.
func lockUpdate(path string) (func(), bool) {
	if _, busy := updateLocks.LoadOrStore(path, true); busy {
		return nil, false
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		updateLocks.Delete(path)
		return nil, false
	}
	return func() { f.Close(); updateLocks.Delete(path) }, true
}

func waitExit(pid string, limit time.Duration) {}
