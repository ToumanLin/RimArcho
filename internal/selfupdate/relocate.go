package selfupdate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var workshopPath = regexp.MustCompile(`(?i)(?:^|/)steamapps/workshop/content/294100/[0-9]+(?:/|$)`)

// InWorkshopFolder recognizes any library's RimWorld Workshop item, including
// release archives extracted into a child folder. It does not access the disk.
func InWorkshopFolder(appDir string) (string, bool) {
	normal := filepath.ToSlash(filepath.Clean(appDir))
	normal = strings.ReplaceAll(normal, `\`, "/")
	loc := workshopPath.FindStringIndex(normal)
	if loc == nil {
		return "", false
	}
	return filepath.FromSlash(strings.TrimRight(normal[:loc[1]], "/")), true
}

// ReleaseSize measures only release entries, rejecting links before a copy.
func ReleaseSize(dir string) (int64, error) {
	var size int64
	err := walkRelease(dir, func(_ string, info fs.FileInfo) error {
		if info.Mode().IsRegular() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func walkRelease(dir string, visit func(string, fs.FileInfo) error) error {
	for _, name := range Files {
		root := filepath.Join(dir, name)
		if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
				return fmt.Errorf("release entry is not a plain file or directory: %s", path)
			}
			return visit(path, info)
		}); err != nil {
			return err
		}
	}
	return nil
}

// CopyRelease stages a confirmed relocation on the destination volume. Apply
// supplies rollback when replacing an existing installation; unrelated files stay.
func CopyRelease(from, to string) (swap *Swap, err error) {
	if _, ok := InWorkshopFolder(to); ok {
		return nil, errors.New("choose a folder outside the Steam Workshop")
	}
	if _, err := ReleaseSize(from); err != nil {
		return nil, err
	}
	for _, name := range []string{"archo.exe", "RimArcho.exe"} {
		if st, err := os.Stat(filepath.Join(from, name)); err != nil || !st.Mode().IsRegular() {
			return nil, fmt.Errorf("release is missing %s", name)
		}
	}
	if err := os.MkdirAll(to, 0755); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(to, ".archo-relocate-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(stage)
		}
	}()
	err = walkRelease(from, func(path string, info fs.FileInfo) error {
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(stage, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}

		n, err := io.Copy(out, in)
		if err == nil && n != info.Size() {
			err = fmt.Errorf("release file changed while copying: %s", path)
		}
		return errors.Join(err, out.Close())
	})
	if err != nil {
		return nil, err
	}
	old := stage + "-old"
	swap, err = Apply(to, stage, old)
	return swap, err
}

// ItemFiles are the release entries the published RimArcho Workshop item carries at its
// own root (17 §5.1 Upload): Monitor's LICENSE, and the READMEs and agent skills that
// `archo workshop publish` copies beside the zip. A release extracted into the item root
// overwrites them with the same files, so cleanup there must keep them.
var ItemFiles = []string{"README.md", "README.zh-CN.md", "LICENSE", "skills"}

// CleanWorkshopRelease removes only the extracted release entries. The caller
// waits for the old process to exit; the item directory itself is never removed.
// At the item root the entries the item ships itself (ItemFiles) stay; in a folder
// below it, where the item has none of them, every release entry is the extraction's.
func CleanWorkshopRelease(dir string) error {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}

	item, ok := InWorkshopFolder(real)
	if !ok {
		return errors.New("refusing to clean a redirected Workshop folder")
	}
	dir = real
	atRoot := strings.EqualFold(filepath.Clean(item), filepath.Clean(dir))

	if _, err := ReleaseSize(dir); err != nil {
		return err
	}
	var errs []error
	for _, name := range Files {
		if atRoot && slices.Contains(ItemFiles, name) {
			continue
		}
		errs = append(errs, os.RemoveAll(filepath.Join(dir, name)))
	}
	return errors.Join(errs...)
}
