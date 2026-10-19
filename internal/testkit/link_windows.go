package testkit

import (
	"os/exec"
	"path/filepath"
)

// Junction creates a directory junction link → target (mklink /J needs no privileges).
func (e *Env) Junction(link, target string) {
	e.T.Helper()
	e.MkdirAll(filepath.Dir(link))
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		e.T.Fatalf("mklink /J %s %s: %v\n%s", link, target, err, out)
	}
}
