package testkit

import (
	"os/exec"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/testkit/hide"
)

// Junction creates a directory junction link → target (mklink /J needs no privileges).
// The cmd child gets no console window (see hide.Cmd).
func (e *Env) Junction(link, target string) {
	e.T.Helper()
	e.MkdirAll(filepath.Dir(link))
	if out, err := hide.Cmd(exec.Command("cmd", "/c", "mklink", "/J", link, target)).CombinedOutput(); err != nil {
		e.T.Fatalf("mklink /J %s %s: %v\n%s", link, target, err, out)
	}
}
