//go:build !windows

package source

import (
	"golang.org/x/sys/unix"
	"os"
)

func unlinkLink(root *os.Root, name string) error {
	// Open the pinned root's directory; /proc is not needed and this never follows
	// the final entry. os.Root pins ancestors against path traversal.
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	return unix.Unlinkat(int(f.Fd()), name, 0)
}
