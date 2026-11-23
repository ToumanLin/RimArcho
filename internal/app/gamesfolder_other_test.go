//go:build !windows

package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestGamesFolderByMount(t *testing.T) {
	t.Parallel()
	mounts := map[string]string{"/home/p": "/home", "/home/p/.steam/steam/steamapps/common/RimWorld": "/home",
		"/mnt/games/SteamLibrary/steamapps/common/RimWorld": "/mnt/games", "/media/ro/RimWorld": "/media/ro",
		"/opt/RimWorld": "/"}
	mountRoot := func(p string) (string, error) {
		if m, ok := mounts[p]; ok {
			return m, nil
		}
		return "", errors.New("unreadable")
	}
	writable := func(dir string) bool { return !strings.HasPrefix(dir, "/media/ro") }
	for _, c := range []struct {
		roots []string
		want  string
	}{
		{nil, "/home/p/Games"},
		{[]string{"/mnt/games/SteamLibrary/steamapps/common/RimWorld"}, "/mnt/games/Games"},
		{[]string{"/home/p/.steam/steam/steamapps/common/RimWorld", "/mnt/games/SteamLibrary/steamapps/common/RimWorld"}, "/home/p/Games"},
		{[]string{"", "/gone/RimWorld", "/mnt/games/SteamLibrary/steamapps/common/RimWorld"}, "/mnt/games/Games"},
		{[]string{"/media/ro/RimWorld"}, "/home/p/Games"},
		{[]string{"/opt/RimWorld"}, "/home/p/Games"},
	} {
		if got := gamesFolderAt(c.roots, "/home/p", mountRoot, writable); got != filepath.FromSlash(c.want) {
			t.Errorf("%v: %q, want %q", c.roots, got, c.want)
		}
	}
}

func TestGamesFolderLive(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if got := GamesFolder([]string{root}); !strings.HasSuffix(got, "/Games") || !filepath.IsAbs(got) {
		t.Fatalf("GamesFolder: %q", got)
	}
}
