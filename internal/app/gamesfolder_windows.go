package app

import (
	"os"
	"path/filepath"
)

// GamesFolder suggests where a downloaded or copied game goes (16 §2.15): <drive>\Games on
// the drive of the first game root, else on the system drive.
func GamesFolder(gameRoots []string) string {
	for _, r := range gameRoots {
		if v := filepath.VolumeName(r); v != "" {
			return v + `\Games`
		}
	}
	if v := os.Getenv("SystemDrive"); v != "" {
		return v + `\Games`
	}
	return filepath.Join(os.Getenv("HOME"), "Games")
}
