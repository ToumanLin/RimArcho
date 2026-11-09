//go:build windows

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/surface/gui"
)

// runGUI opens the GUI. `archo gui` hands over to RimArcho.exe next to it when there is
// one (no console stays open), else it runs the GUI in this process.
func runGUI(ap *app.App, fromConsole bool) error {
	if fromConsole && runtime.GOOS == "windows" {
		if exe, err := os.Executable(); err == nil {
			sib := filepath.Join(filepath.Dir(exe), "RimArcho.exe")
			if !strings.EqualFold(sib, exe) {
				if _, err := os.Stat(sib); err == nil {
					var args []string
					if ap.EnvName != "" {
						args = []string{"--env", ap.EnvName}
					}
					c := exec.Command(sib, args...)
					c.Env = append(os.Environ(), "RIMARCHO_HOME="+ap.Home)
					return c.Start()
				}
			}
		}
	}
	if err := guiPreflight(); err != nil {
		return err
	}
	return gui.Run(gui.Options{App: ap})
}
