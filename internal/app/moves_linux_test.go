package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
)

func TestMoveDirAcrossLinuxMounts(t *testing.T) {
	t.Parallel()
	from := t.TempDir()
	other, err := os.MkdirTemp("/dev/shm", "rimarcho-safety-")
	if err != nil {
		t.Skip("no writable second filesystem:", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(other) })
	if source.SameVolume(from, other) {
		t.Skip("/dev/shm is on the temp volume")
	}
	file := filepath.Join(from, "payload.txt")
	if err := os.WriteFile(file, []byte("checked copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	to := filepath.Join(other, "moved")
	trashed := false
	a := &App{
		trashAllow: func(string) error { return nil },
		TrashFunc: func(path string) error {
			b, err := os.ReadFile(filepath.Join(to, "payload.txt"))
			if err != nil || string(b) != "checked copy" {
				t.Fatalf("recycled before verified copy: %q %v", b, err)
			}
			trashed = true
			return os.RemoveAll(path)
		},
	}
	if err := a.moveDir(from, to); err != nil {
		t.Fatal(err)
	}
	if !trashed {
		t.Fatal("cross-mount move did not recycle the original")
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatal("original remains:", err)
	}
}
