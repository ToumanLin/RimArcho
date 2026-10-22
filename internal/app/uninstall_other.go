//go:build !windows

package app

import (
	"os"
	"path/filepath"
)

func documentsDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Documents")
}

// archoProcesses is not implemented outside Windows: none are found.
func archoProcesses() []*GameRun { return nil }

// startAfterExit deletes at once: outside Windows a running program and an open database
// can be deleted.
func startAfterExit(x AfterExit, log string) error {
	RunAfterExit(x)
	return nil
}
