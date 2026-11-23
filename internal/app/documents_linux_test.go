package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLinuxUserDirsDocuments(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "user-dirs.dirs")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, c := range []struct{ body, want string }{
		{"# written by xdg-user-dirs-update\nXDG_DESKTOP_DIR=\"$HOME/Desktop\"\nXDG_DOCUMENTS_DIR=\"$HOME/Dokumenty\"\n", "/home/p/Dokumenty"},
		{"XDG_DOCUMENTS_DIR=\"/data/My Documents/\"\n", "/data/My Documents"},
		{"XDG_DOCUMENTS_DIR=\"$HOME\"\n", "/home/p"},
		{"XDG_DOCUMENTS_DIR=relative\nXDG_DOCUMENTS_DIR=\"docs\"\n", ""},
		{"XDG_DESKTOP_DIR=\"$HOME/Desktop\"\n", ""},
		{"XDG_DOCUMENTS_DIR=\"$HOME/A\"\nXDG_DOCUMENTS_DIR=\"$HOME/B\"\n", "/home/p/B"},
	} {
		if got := userDirsDocuments(write(c.body), "/home/p"); got != c.want {
			t.Errorf("%q: %q, want %q", c.body, got, c.want)
		}
	}
	if got := userDirsDocuments(filepath.Join(dir, "missing"), "/home/p"); got != "" {
		t.Fatalf("missing file: %q", got)
	}
}

func TestLinuxDocumentsDir(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "cfg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	if got := documentsDir(); got != filepath.Join(home, "Documents") {
		t.Fatalf("without user-dirs.dirs: %q", got)
	}
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "user-dirs.dirs"), []byte("XDG_DOCUMENTS_DIR=\"$HOME/Dokumente\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := documentsDir(); got != filepath.Join(home, "Dokumente") {
		t.Fatalf("from XDG_CONFIG_HOME: %q", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "relative")
	if got := documentsDir(); got != filepath.Join(home, "Documents") {
		t.Fatalf("a relative XDG_CONFIG_HOME falls back to ~/.config: %q", got)
	}
}

// TestLinuxLogBirthTime: a log whose mtime is later than its creation starts at its
// birth time where the file system records one (ext4, btrfs, xfs, tmpfs), else at its mtime.
func TestLinuxLogBirthTime(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "Player.log")
	before := time.Now().Add(-2 * time.Second)
	if err := os.WriteFile(path, []byte("no Started line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	born := logCreationTime(path, fi)
	got := logStart(path, fi)
	if born.IsZero() {
		if !got.Equal(fi.ModTime()) {
			t.Fatalf("without a birth time: %v, want the mtime %v", got, fi.ModTime())
		}
		t.Skip("this file system records no birth time")
	}
	if born.Before(before) || born.After(time.Now().Add(2*time.Second)) || !got.Equal(born) {
		t.Fatalf("birth %v, start %v (written after %v)", born, got, before)
	}
	if !logCreationTime(filepath.Join(filepath.Dir(path), "missing"), fi).IsZero() {
		t.Fatal("a missing file has a birth time")
	}
}
