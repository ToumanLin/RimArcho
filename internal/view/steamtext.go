package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// The Steam client signed out by SteamCMD, and offline (VM run B11).
func init() {

	interfaceFormats = append(interfaceFormats, interfaceFormatFor(app.SteamSignOutNote), interfaceFormatFor("note: "+app.SteamSignOutNote))
	// A Workshop download that signs in only for the items anonymous download is denied (11 §3).
	for _, note := range []string{app.WorkshopSignOutNote, app.WorkshopAccountNote} {
		interfaceFormats = append(interfaceFormats, interfaceFormatFor("note: "+note), interfaceFormatFor(note)) // "note: " first: "%d" at the start would take it
	}
}

// SteamCMDPasswordHint is the password hints in one line (11 §3): the TUI's status line.
func SteamCMDPasswordHint() string {
	parts := make([]string, len(app.SteamCMDPasswordHints))
	for i, h := range app.SteamCMDPasswordHints {
		parts[i] = T(h)
	}
	return T("SteamCMD window") + ": " + strings.Join(parts, "; ")
}
