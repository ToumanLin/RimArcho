package view

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// boundedSearchFS refuses linked escapes and skips oversized input instead of
// reading an unbounded file. The viewer has its own, smaller partial-read cap.
type boundedSearchFS struct {
	root, realRoot string
	dirs           map[string]string
}

func newSearchFS(root string) *boundedSearchFS {
	real, _ := filepath.EvalSymlinks(root)
	return &boundedSearchFS{root: filepath.Clean(root), realRoot: real, dirs: map[string]string{filepath.Clean(root): real}}
}

func insideResolved(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Cache resolved parents for this Mod only. Ordinary children need one Lstat;
// links are fully resolved before opening, including junctions on Windows.
func (b *boundedSearchFS) resolve(path string, dir bool) (string, error) {
	path = filepath.Clean(path)
	if !insideResolved(b.root, path) {
		return "", fs.ErrPermission
	}
	if real, ok := b.dirs[path]; ok {
		if real == "" {
			return "", fs.ErrPermission
		}
		return real, nil
	}
	parent, err := b.resolve(filepath.Dir(path), true)
	if err != nil {
		return "", err
	}
	real := filepath.Join(parent, filepath.Base(path))
	info, err := os.Lstat(real)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		real, err = filepath.EvalSymlinks(real)
		if err != nil {
			return "", err
		}
	}
	if !insideResolved(b.realRoot, real) {
		return "", fs.ErrPermission
	}
	if dir {
		b.dirs[path] = real
	}
	return real, nil
}

func (b *boundedSearchFS) ReadDir(path string) ([]fs.DirEntry, error) {
	real, err := b.resolve(path, true)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(real)
}
func (b *boundedSearchFS) ReadFile(path string) ([]byte, error) {
	real, err := b.resolve(path, false)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(real)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, FileSearchReadCap+1))
	if len(data) > FileSearchReadCap {
		return nil, errors.New(T("file exceeds the 2 MiB search limit; open it to search further"))
	}
	return data, err
}
