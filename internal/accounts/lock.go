// Package accounts owns player-only sessions. Secret values never leave this package
// except as HTTP cookies sent to their originating service.
package accounts

import (
	"path/filepath"
	"strings"
	"sync"
)

var locks sync.Map

// Lock serializes a service instance across application read sessions.
func Lock(instance string) func() {
	if abs, err := filepath.Abs(instance); err == nil {
		instance = abs
	}
	if real, err := filepath.EvalSymlinks(instance); err == nil {
		instance = real
	}
	instance = strings.ToLower(filepath.Clean(instance))
	m, _ := locks.LoadOrStore(instance, &sync.Mutex{})
	m.(*sync.Mutex).Lock()
	unlock := platformLock(instance)
	return func() { unlock(); m.(*sync.Mutex).Unlock() }
}
