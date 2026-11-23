//go:build !windows

package app

import (
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/source"
	"golang.org/x/sys/unix"
)

// GamesFolder suggests where a downloaded or copied game goes (16 §2.15): <mount>/Games
// when the first game root is on a writable file system other than the one
// holding the home folder and the root file system (a second disk such as /mnt/games),
// else ~/Games.
func GamesFolder(gameRoots []string) string {
	home, _ := os.UserHomeDir()
	return gamesFolderAt(gameRoots, home, source.MountRoot, func(dir string) bool { return unix.Access(dir, unix.W_OK) == nil })
}

func gamesFolderAt(gameRoots []string, home string, mountRoot func(string) (string, error), writable func(string) bool) string {
	fallback := filepath.Join(home, "Games")
	homeMount, err := mountRoot(home)
	if err != nil {
		homeMount = ""
	}
	for _, r := range gameRoots {
		if r == "" {
			continue
		}
		mount, err := mountRoot(r)
		if err != nil {
			continue
		}
		if mount == "/" || mount == homeMount || !writable(mount) {
			return fallback
		}
		return filepath.Join(mount, "Games")
	}
	return fallback
}
