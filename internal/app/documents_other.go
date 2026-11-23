//go:build !windows && !linux

package app

import (
	"os"
	"path/filepath"
)

func documentsDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Documents")
}
