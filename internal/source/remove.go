package source

import (
	"os"
	"path/filepath"
)

// RemoveTree removes only this entry's tree. Reparse points and symbolic links
// inside it are unlinked without enumerating their targets.
func RemoveTree(path string) error {
	if IsLink(path) {
		return RemoveLink(path)
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.Remove(path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := RemoveTree(filepath.Join(path, e.Name())); err != nil {
			return err
		}
	}
	return os.Remove(path)
}
