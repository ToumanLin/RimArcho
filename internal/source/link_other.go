//go:build !windows

package source

import (
	"fmt"
	"os"
	"path/filepath"
)

// IsLink reports whether p is a symbolic link.
func IsLink(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// RealPath resolves symbolic links.
func RealPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// CreateLink makes link a symbolic link to target (09 §8).
func CreateLink(link, target string) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	return os.Symlink(abs, link)
}

// RemoveLink deletes a symbolic link itself, never its target.
func RemoveLink(link string) error {
	if !IsLink(link) {
		return fmt.Errorf("%s is not a link; refusing to delete it", link)
	}
	return os.Remove(link)
}
