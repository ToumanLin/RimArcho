//go:build !windows

package testkit

import (
	"os"
	"path/filepath"
)

// Junction creates a directory symbolic link (the non-Windows stand-in for a junction).
func (e *Env) Junction(link, target string) {
	e.T.Helper()
	e.MkdirAll(filepath.Dir(link))
	if err := os.Symlink(target, link); err != nil {
		e.T.Fatal(err)
	}
}
