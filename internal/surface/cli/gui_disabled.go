//go:build !windows && !(linux && cgo)

package cli

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// runGUI in a build without the window (the static Linux archo): `archo gui` hands over to
// rimarcho next to it when there is one (16 §8.1), else it says there is no window.
func runGUI(ap *app.App, fromConsole bool) error {
	if fromConsole {
		if sib := guiSibling(); sib != "" {
			return handOver(ap, sib)
		}
	}
	view.SetLang(cmds.Setting(ap, "ui.lang"))
	return &windowUnavailable{view.T("This build has no window; the text interface (archo) has the same features.")}
}
