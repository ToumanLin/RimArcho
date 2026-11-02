//go:build !windows

package app

import (
	"os"

	"golang.org/x/sys/unix"
)

func sourceLockBusy(err error) bool { return err == unix.EWOULDBLOCK || err == unix.EAGAIN }

func lockSourceLibraryFile(path string, exclusive bool) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return nil, err
	}
	flag := unix.LOCK_SH
	if exclusive {
		flag = unix.LOCK_EX
	}
	if err := unix.Flock(int(f.Fd()), flag|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return func() { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }, nil
}
