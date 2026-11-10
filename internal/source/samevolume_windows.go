package source

import (
	"path/filepath"
	"strings"
)

// SameVolume reports whether a move can use rename. Unknown volumes use copying.
func SameVolume(from, to string) bool {
	return strings.EqualFold(filepath.VolumeName(from), filepath.VolumeName(to))
}
