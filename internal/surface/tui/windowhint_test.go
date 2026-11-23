package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// The "open the window" hint names the window program of the platform beside archo: rimarcho
// on Linux, RimArcho.exe on Windows (16 §9.2).
func TestWindowNextTo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, n := range []string{"archo", "archo.exe", "rimarcho", "RimArcho.exe"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ exe, goos, want string }{
		{"archo", "linux", "rimarcho"},
		{"archo.exe", "windows", "RimArcho.exe"},
		{"rimarcho", "linux", ""},
		{"RimArcho.exe", "windows", ""},
		{"archo", "darwin", ""},
	} {
		if got := windowNextTo(filepath.Join(dir, c.exe), c.goos); got != c.want {
			t.Errorf("%s on %s: %q, want %q", c.exe, c.goos, got, c.want)
		}
	}
	empty := t.TempDir()
	if got := windowNextTo(filepath.Join(empty, "archo"), "linux"); got != "" {
		t.Errorf("no window program: %q", got)
	}
}
