//go:build !windows

package app

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockGameLinks(mods string) (func(), error) {
	f, err := os.Open(mods)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return func() { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }, nil
}
