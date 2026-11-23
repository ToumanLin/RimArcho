package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// guiProgram is the GUI program's file name on this platform (16 §8.1): RimArcho.exe on
// Windows, rimarcho on Linux, none elsewhere.
func guiProgram(goos string) string {
	switch goos {
	case "windows":
		return "RimArcho.exe"
	case "linux":
		return "rimarcho"
	}
	return ""
}

// guiSibling is the GUI program next to this executable, or "" when there is none or this is
// it. `archo gui` hands over to it.
func guiSibling() string {
	name := guiProgram(runtime.GOOS)
	exe, err := os.Executable()
	if name == "" || err != nil {
		return ""
	}
	sib := filepath.Join(filepath.Dir(exe), name)
	if strings.EqualFold(sib, exe) {
		return ""
	}
	if fi, err := os.Stat(sib); err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	return sib
}

// handOver starts the GUI program with this Game Config and data folder and returns at
// once. On Linux it gets its own session, so closing the terminal does not hang it up.
func handOver(ap *app.App, sib string) error {
	var args []string
	if ap.EnvName != "" {
		args = []string{"--env", ap.EnvName}
	}
	c := exec.Command(sib, args...)
	c.Env = append(os.Environ(), "RIMARCHO_HOME="+ap.Home)
	ownSession(c)
	return c.Start()
}
