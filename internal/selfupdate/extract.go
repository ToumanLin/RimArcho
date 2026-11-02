package selfupdate

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Files lists the top-level entries replaced for the current operating system.
var Files = releaseFiles(runtime.GOOS)

func releaseFiles(goos string) []string {
	programs := []string{"archo"}
	if goos == "windows" {
		programs = []string{"archo.exe", "RimArcho.exe"}
	}
	return append(programs, "README.md", "CHANGELOG.md", "LICENSE", "monitor")
}

// Extract validates and unpacks a release into a new directory, removing it on failure.
func Extract(zipPath, dir string) (err error) {
	if err := os.Mkdir(dir, 0o755); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer z.Close()
	var total uint64
	names := make([]string, len(z.File))
	for i, entry := range z.File {
		name, err := extractName(entry.Name)
		if err != nil {
			return err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("zip entry %q is a symlink", entry.Name)
		}
		if entry.UncompressedSize64 > maxDownload {
			return fmt.Errorf("zip entry %q exceeds 200 mb", entry.Name)
		}
		total += entry.UncompressedSize64
		if total > 500<<20 {
			return errors.New("extracted zip exceeds 500 mb")
		}
		names[i] = name
	}
	for i, entry := range z.File {
		name := names[i]
		target := filepath.Join(dir, filepath.FromSlash(name))
		if entry.FileInfo().IsDir() || strings.HasSuffix(entry.Name, `\`) {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractFile(entry, target); err != nil {
			return err
		}
	}
	programs := []string{"archo"}
	if runtime.GOOS == "windows" {
		programs = []string{"archo.exe", "RimArcho.exe"}
	}
	for _, program := range programs {
		info, err := os.Stat(filepath.Join(dir, program))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("release zip is missing program %q", program)
		}
	}
	return nil
}

func extractName(name string) (string, error) {
	normal := strings.ReplaceAll(name, `\`, "/")
	if normal == "" || strings.HasPrefix(normal, "/") || strings.Contains(normal, ":") || strings.ContainsRune(normal, 0) {
		return "", fmt.Errorf("unsafe zip path %q", name)
	}
	for _, part := range strings.Split(normal, "/") {
		if part == ".." {
			return "", fmt.Errorf("unsafe zip path %q", name)
		}
		// Windows trims these suffixes, which could otherwise alias a rejected path.
		if part != "." && (strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ")) {
			return "", fmt.Errorf("unsafe zip path %q", name)
		}
	}
	normal = path.Clean(normal)
	top, rest, nested := strings.Cut(normal, "/")
	for _, allowed := range Files {
		if strings.EqualFold(top, allowed) {
			if nested {
				return allowed + "/" + rest, nil
			}
			return allowed, nil
		}
	}
	return "", fmt.Errorf("zip has unexpected top-level entry %q", top)
}

func extractFile(entry *zip.File, target string) error {
	r, err := entry.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	mode := os.FileMode(0o666)
	if runtime.GOOS != "windows" {
		mode = 0o755
	}
	w, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(w, io.LimitReader(r, maxDownload+1))
	closeErr := w.Close()
	if n > maxDownload {
		return errors.Join(errors.New("zip entry exceeds 200 mb"), closeErr)
	}
	return errors.Join(copyErr, closeErr)
}
