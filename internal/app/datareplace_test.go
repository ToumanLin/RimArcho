package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataReplacementRollback(t *testing.T) {
	t.Parallel()
	names := []string{"rules", "steamdb", "replacements", "rules.source", "steamdb.source", "steamdb.index", "replacements.source"}
	for _, existing := range []bool{false, true} {
		steps := len(names)
		if existing {
			steps *= 2 // backing up the old set, then publishing the new set
		}
		for failAt := 1; failAt <= steps+1; failAt++ {
			t.Run(fmt.Sprintf("existing=%v/failure=%d", existing, failAt), func(t *testing.T) {
				dir := t.TempDir()
				var files []stagedDataFile
				for _, name := range names {
					dst := filepath.Join(dir, name)
					if existing {
						if err := os.WriteFile(dst, []byte("old "+name), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					file, err := stageDataFile(dst, []byte("new "+name))
					if err != nil {
						t.Fatal(err)
					}
					files = append(files, file)
				}
				injected := errors.New("synthetic disk failure")
				calls, commits := 0, 0
				rename := func(src, dst string) error {
					calls++
					if failAt <= steps && calls == failAt {
						return injected
					}
					return os.Rename(src, dst)
				}
				err := replaceDataFiles(files, rename, func() error { commits++; return injected })
				if !errors.Is(err, injected) || (failAt <= steps && commits != 0) || (failAt > steps && commits != 1) {
					t.Fatal(err, commits)
				}
				for _, name := range names {
					b, err := os.ReadFile(filepath.Join(dir, name))
					if existing && (err != nil || string(b) != "old "+name) {
						t.Fatalf("%s not restored: %q %v", name, b, err)
					}
					if !existing && !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("partial initial install: %s %q %v", name, b, err)
					}
				}
				left, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range left {
					if strings.HasPrefix(entry.Name(), ".community-data-backup-") {
						t.Fatal("rollback backup leaked", entry.Name())
					}
				}
			})
		}
	}
}

func TestDataReplacementSuccess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dst := filepath.Join(dir, "rules")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := stageDataFile(dst, []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	if err := replaceDataFiles([]stagedDataFile{file}, os.Rename, func() error { committed = true; return nil }); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != "new" || !committed {
		t.Fatal(string(b), err, committed)
	}
	left, err := os.ReadDir(dir)
	if err != nil || len(left) != 1 {
		t.Fatal(left, err)
	}
}
