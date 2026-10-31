package selfupdate

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type zipEntry struct {
	name string
	data string
	mode fs.FileMode
	size uint64
}

func makeZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, entry := range entries {
		h := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		if entry.mode != 0 {
			h.SetMode(entry.mode)
		}
		if entry.size != 0 {
			h.UncompressedSize64 = entry.size
			if _, err := z.CreateRaw(h); err != nil {
				t.Fatal(err)
			}
			continue
		}
		w, err := z.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entry.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func programEntries() []zipEntry {
	entries := []zipEntry{{name: Files[0], data: "command program"}}
	if runtime.GOOS == "windows" {
		entries = append(entries, zipEntry{name: "RimArcho.exe", data: "gui program"})
	}
	return entries
}

func writeZip(t *testing.T, root string, entries []zipEntry) string {
	t.Helper()
	name := filepath.Join(root, "release.zip")
	if err := os.WriteFile(name, makeZip(t, entries), 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestExtract(t *testing.T) {
	root := t.TempDir()
	entries := programEntries()
	entries[0].name = strings.ToUpper(entries[0].name)
	entries = append(entries, zipEntry{name: "readme.MD", data: "read me"},
		zipEntry{name: `MONITOR\`, mode: os.ModeDir | 0o755},
		zipEntry{name: `MONITOR\ArchoMonitor\About\About.xml`, data: "<About/>"})
	zipPath := writeZip(t, root, entries)
	dir := filepath.Join(root, "stage")
	if err := Extract(zipPath, dir); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{Files[0]: "command program", "README.md": "read me", "monitor/ArchoMonitor/About/About.xml": "<About/>"}
	if runtime.GOOS == "windows" {
		want["RimArcho.exe"] = "gui program"
	}
	if got := tree(t, dir); !equalTrees(got, want) {
		t.Fatalf("tree = %v, want %v", got, want)
	}
	if runtime.GOOS != "windows" {
		for name := range want {
			info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name)))
			if err != nil || info.Mode().Perm() != 0o755 {
				t.Errorf("%s mode: %v, %v", name, info, err)
			}
		}
	}
	if err := Extract(zipPath, dir); err == nil {
		t.Fatal("accepted existing directory")
	}
	if got := tree(t, dir); !equalTrees(got, want) {
		t.Fatal("existing directory changed")
	}
}

func TestExtractRejectsUnsafeArchives(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []zipEntry
		want    string
	}{
		{"zip slip", []zipEntry{{name: `..\evil`, data: "bad"}}, "unsafe zip path"},
		{"nested zip slip", []zipEntry{{name: `monitor\..\evil`, data: "bad"}}, "unsafe zip path"},
		{"absolute", []zipEntry{{name: "/evil", data: "bad"}}, "unsafe zip path"},
		{"windows absolute", []zipEntry{{name: `\evil`, data: "bad"}}, "unsafe zip path"},
		{"drive", []zipEntry{{name: `C:\evil`, data: "bad"}}, "unsafe zip path"},
		{"drive relative", []zipEntry{{name: `C:evil`, data: "bad"}}, "unsafe zip path"},
		{"unc", []zipEntry{{name: `\\host\evil`, data: "bad"}}, "unsafe zip path"},
		{"unknown", []zipEntry{{name: "evil.exe", data: "bad"}}, "unexpected top-level"},
		{"symlink", []zipEntry{{name: "monitor/link", data: "outside", mode: os.ModeSymlink | 0o777}}, "symlink"},
		{"large file", []zipEntry{{name: "monitor/large", size: maxDownload + 1}}, "exceeds 200 mb"},
		{"large total", []zipEntry{{name: "monitor/a", size: 180 << 20}, {name: "monitor/b", size: 180 << 20}, {name: "monitor/c", size: 180 << 20}}, "exceeds 500 mb"},
		{"missing program", nil, "missing program"},
		{"program is directory", []zipEntry{{name: Files[0] + "/", mode: os.ModeDir | 0o755}}, "missing program"},
		{"duplicate", []zipEntry{{name: "README.md", data: "a"}, {name: "readme.md", data: "b"}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			entries := programEntries()
			if tc.name == "missing program" {
				entries = entries[:len(entries)-1]
			} else if tc.name == "program is directory" {
				entries = nil
			}
			entries = append(entries, tc.entries...)
			dir := filepath.Join(root, "stage")
			err := Extract(writeZip(t, root, entries), dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed extraction left stage: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "evil")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("extraction escaped stage")
			}
		})
	}
}

func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func equalTrees(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for name, value := range a {
		if other, ok := b[name]; !ok || other != value {
			return false
		}
	}
	return true
}
