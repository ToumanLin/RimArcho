//go:build !windows

package source

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// Trash moves a file or folder to the user's trash (10 §2.4): ~/.Trash on macOS, the
// freedesktop trash elsewhere. A move across volumes is refused, never turned into a delete.
func Trash(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	name := filepath.Base(abs)
	if runtime.GOOS == "darwin" {
		dst := uniqueIn(filepath.Join(home, ".Trash"), name)
		return os.Rename(abs, dst)
	}
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		root = filepath.Join(home, ".local", "share")
	}
	files, info := filepath.Join(root, "Trash", "files"), filepath.Join(root, "Trash", "info")
	if err := os.MkdirAll(files, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(info, 0o700); err != nil {
		return err
	}
	dst := uniqueIn(files, name)
	meta := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n", (&url.URL{Path: abs}).EscapedPath(), time.Now().Format("2006-01-02T15:04:05"))
	infoPath := filepath.Join(info, filepath.Base(dst)+".trashinfo")
	if err := os.WriteFile(infoPath, []byte(meta), 0o600); err != nil {
		return err
	}
	if err := os.Rename(abs, dst); err != nil {
		os.Remove(infoPath)
		return fmt.Errorf("cannot move %s to the trash: %v", abs, err)
	}
	return nil
}

func uniqueIn(dir, name string) string {
	p := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return p
		}
		p = filepath.Join(dir, name+"."+strconv.Itoa(i))
	}
}
