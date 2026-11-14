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
	"slices"
	"strings"
)

// Files lists the top-level entries every release up to 1.0.x installed. RimArcho before
// 1.1 rejects a zip with other top-level entries, so a release that 1.0.x must install
// keeps to these. From 1.1 on, Extract accepts any safe top-level name and the install
// records what it put in place (Installed, WriteManifest), so later releases may add or
// drop entries freely; Files stays the baseline for installations without that record.
var Files = releaseFiles(runtime.GOOS)

func releaseFiles(goos string) []string {
	programs := []string{"archo"}
	if goos == "windows" {
		programs = []string{"archo.exe", "RimArcho.exe"}
	}
	return append(programs, "README.md", "README.zh-CN.md", "CHANGELOG.md", "LICENSE", "monitor", "skills")
}

// Layout says which top-level entries a zip may have and which files it must contain.
type Layout struct {
	Files    []string // allowed top-level entries (files or folders), matched without case; nil allows any safe name
	Required []string // regular files that must exist after extraction, slash-separated
}

// Extract validates and unpacks a release into a new directory, removing it on failure.
func Extract(zipPath, dir string) error {
	programs := []string{"archo"}
	if runtime.GOOS == "windows" {
		programs = []string{"archo.exe", "RimArcho.exe"}
	}
	return ExtractLayout(zipPath, dir, Layout{Required: programs})
}

// ExtractLayout is Extract for another zip's layout (the Archo Agent bundle, 15 §7).
// A root entry "./" (Windows tar writes one) is skipped.
func ExtractLayout(zipPath, dir string, layout Layout) (err error) {
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
		name, err := extractName(entry.Name, layout.Files)
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
		if name == "" {
			if err := checkSkippedEntry(entry); err != nil {
				return err
			}
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if entry.FileInfo().IsDir() || strings.HasSuffix(entry.Name, `\`) {
			if err := checkSkippedEntry(entry); err != nil {
				return err
			}
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
	for _, file := range layout.Required {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(file)))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("release zip is missing program %q", file)
		}
	}
	return nil
}

// Even entries with no output must be read to EOF for archive/zip's CRC check.
func checkSkippedEntry(entry *zip.File) error {
	r, err := entry.Open()
	if err != nil {
		return err
	}
	n, readErr := io.Copy(io.Discard, io.LimitReader(r, maxDownload+1))
	if n > maxDownload {
		readErr = errors.Join(readErr, errors.New("zip entry exceeds 200 mb"))
	}
	return errors.Join(readErr, r.Close())
}

// extractName returns the entry's path under the allowed top-level name, or "" for the zip's root folder.
func extractName(name string, files []string) (string, error) {
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
	if normal == "." {
		return "", nil
	}
	top, rest, nested := strings.Cut(normal, "/")
	if files == nil {
		if !EntryNameOK(top) {
			return "", fmt.Errorf("zip has unsafe top-level entry %q", top)
		}
		files = append(slices.Clone(Files), top) // known names keep their spelling
	}
	for _, allowed := range files {
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
