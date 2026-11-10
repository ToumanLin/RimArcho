package app

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// renameNoReplace renames from to to, failing with os.ErrExist when to exists: MoveFileEx
// without MOVEFILE_REPLACE_EXISTING (os.Rename passes it). A rename that changes only the
// letter case of the same file works.
func renameNoReplace(from, to string) error {
	f, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	t, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(f, t, 0); err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
			err = os.ErrExist
		}
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}
