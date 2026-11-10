package app

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// RENAME_NOREPLACE keeps a target that appears after the caller's check intact.
// On a case-insensitive filesystem, the target may be the source itself.
func renameNoReplace(from, to string) error {
	err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EEXIST) {
		src, e1 := os.Lstat(from)
		dst, e2 := os.Lstat(to)
		if e1 == nil && e2 == nil && os.SameFile(src, dst) {
			return os.Rename(from, to)
		}
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}
