//go:build !windows

package source

import (
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
