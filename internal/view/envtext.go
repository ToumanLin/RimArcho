package view

import (
	"errors"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// The text of environments and the shared mod folders (03 §2): the environment page, the
// settings page's Mod folders, and the add-environment guide's first step.

// DownloadErrorText is a failed download in the player's words (11 §3): why, then
// SteamCMD's own line.
func DownloadErrorText(err error) string {
	var de *app.DownloadError
	if !errors.As(err, &de) {
		return Humanize(nil, err.Error())
	}
	s := T(de.Why())
	if de.Line != "" {
		s += "  " + Tf("SteamCMD: %s", de.Line)
	}
	return s
}
