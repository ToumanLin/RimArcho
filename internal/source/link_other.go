//go:build !windows

package source

import (
	"os"
	"path/filepath"
)

// IsLink reports whether p is a symbolic link.
func IsLink(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && isLinkInfo(fi)
}

func isLinkInfo(fi os.FileInfo) bool {
	return fi.Mode()&os.ModeSymlink != 0
}

// RealPath resolves symbolic links.
func RealPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// CreateLink makes link a symbolic link to target (08 §3).
func CreateLink(link, target string) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	return os.Symlink(abs, link)
}

// RemoveLink deletes a symbolic link itself, never its target.
func RemoveLink(link string) error {
	l, err := ReadLink(link)
	if err != nil {
		return err
	}
	return RemoveLinkChecked(l)
}

func rawLink(path string) (string, string, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return "unknown", "", err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return "plain", "", nil
	}
	target, err := os.Readlink(path)
	return "symlink", target, err
}

func removeLinkChecked(l *LinkInfo) error {
	if err := l.verify(); err != nil {
		return err
	}
	root, err := os.OpenRoot(l.physicalParent)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := l.verify(); err != nil {
		return err
	}
	// unlinkat cannot remove a real directory, including one swapped in after verify.
	return unlinkLink(root, filepath.Base(l.Path))
}
