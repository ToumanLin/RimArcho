//go:build !windows

package source

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

// Trash moves a file or folder to the user's trash (02 §3): ~/.Trash on macOS, the
// freedesktop trash elsewhere. Each entry stays on its original filesystem.
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
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		data = filepath.Join(home, ".local", "share")
	}
	// Resolve the parent, preserving a leaf link as the item being moved.
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return err
	}
	abs = filepath.Join(parent, name)
	root, original := filepath.Join(data, "Trash"), abs
	if !SameVolume(parent, root) {
		mount, err := trashMount(parent)
		if err != nil {
			return err
		}
		root = filepath.Join(mount, ".Trash-"+strconv.Itoa(os.Geteuid()))
		original, err = filepath.Rel(mount, abs)
		if err != nil {
			return err
		}
	}
	if Within(abs, root) {
		return fmt.Errorf("%s is already inside the trash", abs)
	}
	if err := privateTrashDir(root); err != nil {
		return err
	}
	files, info := filepath.Join(root, "files"), filepath.Join(root, "info")
	for _, dir := range []string{files, info} {
		if err := privateTrashDir(dir); err != nil {
			return err
		}
	}
	meta := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n", (&url.URL{Path: original}).EscapedPath(), time.Now().Format("2006-01-02T15:04:05"))
	for n := 1; ; n++ {
		entry := name
		if n > 1 {
			entry += "." + strconv.Itoa(n)
		}
		dst := filepath.Join(files, entry)
		if _, err := os.Lstat(dst); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		infoPath := filepath.Join(info, entry+".trashinfo")
		// Reserve the name before moving, including orphaned metadata entries.
		f, err := os.OpenFile(infoPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		_, err = f.WriteString(meta)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(abs, dst)
		}
		if err != nil {
			os.Remove(infoPath)
			return fmt.Errorf("cannot move %s to the trash: %w", abs, err)
		}
		return nil
	}
}

// trashMount finds the top directory on this device without invoking mount tools.
func trashMount(path string) (string, error) {
	dev, err := deviceID(path)
	if err != nil {
		return "", err
	}
	for filepath.Dir(path) != path {
		parent := filepath.Dir(path)
		d, err := deviceID(parent)
		if err != nil {
			return "", err
		}
		if d != dev {
			break
		}
		path = parent
	}
	return path, nil
}

func privateTrashDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("trash directory %s must be a directory owned by this user with mode 0700", path)
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
