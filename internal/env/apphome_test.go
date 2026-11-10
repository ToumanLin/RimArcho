package env

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestXDGAppHome(t *testing.T) {
	home, data := t.TempDir(), t.TempDir()
	for _, tc := range []struct {
		home, xdg, want string
	}{
		{home, data, filepath.Join(data, "rimarcho")},
		{home, "", filepath.Join(home, ".local", "share", "rimarcho")},
		{home, "relative/data", filepath.Join(home, ".local", "share", "rimarcho")},
		{"", data, filepath.Join(data, "rimarcho")},
		{"relative/home", data, filepath.Join(data, "rimarcho")},
		{"", "relative/data", ""},
		{"relative/home", "", ""},
	} {
		t.Setenv("HOME", tc.home)
		t.Setenv("USERPROFILE", tc.home) // Exercise the Linux resolver on Windows too.
		t.Setenv("XDG_DATA_HOME", tc.xdg)
		got, err := xdgAppHome()
		if tc.want == "" {
			if err == nil || got != "" || !strings.Contains(err.Error(), "absolute HOME or XDG_DATA_HOME") {
				t.Fatalf("home %q, XDG %q: %q %v", tc.home, tc.xdg, got, err)
			}
		} else if err != nil || got != tc.want || !filepath.IsAbs(got) {
			t.Fatalf("home %q, XDG %q: %q %v; want %q", tc.home, tc.xdg, got, err, tc.want)
		}
	}
	t.Setenv("RIMARCHO_HOME", data)
	if got, err := AppHome(); err != nil || got != data {
		t.Fatalf("explicit home: %q %v", got, err)
	}
}
