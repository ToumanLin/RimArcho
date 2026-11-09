//go:build !windows

package cli

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func runGUI(ap *app.App, fromConsole bool) error {
	view.SetLang(cmds.Setting(ap, "ui.lang"))
	return &windowUnavailable{view.T("This build has no window; the text interface (archo) has the same features.")}
}
