//go:build !windows && !linux

package app

import (
	"errors"

	"golang.org/x/sys/unix"
)

// processStart is when process pid started (0: unknown here) and whether it still runs.
func processStart(pid uint32) (int64, bool) {
	if pid == 0 {
		return 0, false
	}
	err := unix.Kill(int(pid), 0)
	return 0, err == nil || errors.Is(err, unix.EPERM)
}

// archoProcesses is not implemented here: none are found.
func archoProcesses() []*GameRun { return nil }
