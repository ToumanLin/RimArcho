//go:build windows || (linux && cgo)

package cli

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/surface/gui"
)

// runGUI opens the GUI. `archo gui` hands over to the GUI program next to it when there is
// one (RimArcho.exe, rimarcho; no console or terminal stays tied to it), else it runs the GUI
// in this process.
func runGUI(ap *app.App, fromConsole bool) error {
	if fromConsole {
		if sib := guiSibling(); sib != "" {
			return handOver(ap, sib)
		}
	}
	if err := guiPreflight(); err != nil {
		return err
	}
	return gui.Run(gui.Options{App: ap})
}
