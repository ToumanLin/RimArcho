//go:build !windows && !linux

package app

import (
	"os"
)

// renameNoReplace renames from to to, failing with os.ErrExist when to is another file.
// Outside Windows a check before the rename is the best portable guard.
func renameNoReplace(from, to string) error {
	if target, err := os.Lstat(to); err == nil {
		original, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if !os.SameFile(original, target) {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: os.ErrExist}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(from, to)
}
