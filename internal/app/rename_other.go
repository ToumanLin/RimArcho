//go:build !windows

package app

import (
	"os"
	"strings"
)

// renameNoReplace renames from to to, failing with os.ErrExist when to is another file.
// Outside Windows a check before the rename is the best portable guard.
func renameNoReplace(from, to string) error {
	if _, err := os.Lstat(to); err == nil && !strings.EqualFold(from, to) {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: os.ErrExist}
	}
	return os.Rename(from, to)
}
