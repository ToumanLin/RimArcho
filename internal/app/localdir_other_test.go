//go:build !windows

package app

import (
	"path/filepath"
	"testing"
)

func TestLocalDirXDG(t *testing.T) {
	home, data := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "wrong-windows-root"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "wrong-cache-root"))
	a := &App{}
	for _, root := range []string{data, "", "relative-not-valid"} {
		t.Setenv("XDG_DATA_HOME", root)
		want := filepath.Join(home, ".local", "share", "rimarcho-local")
		if root == data {
			want = filepath.Join(data, "rimarcho-local")
		}
		if got := a.LocalDir(); got != want {
			t.Fatalf("LocalDir with XDG_DATA_HOME=%q: %s, want %s", root, got, want)
		}
	}
}
