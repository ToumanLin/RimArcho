//go:build !windows

package app

import (
	"os"

	"golang.org/x/sys/unix"
)

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
