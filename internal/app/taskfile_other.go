//go:build !windows

package app

import (
	"errors"
	"os"

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

// lockTaskFile holds the record's lock across processes for one read and write.
func lockTaskFile(path string) func() {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return func() {}
	}
	if unix.Flock(int(f.Fd()), unix.LOCK_EX) != nil {
		f.Close()
		return func() {}
	}
	return func() { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }
}
