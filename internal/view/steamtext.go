package view

import "github.com/ToumanLin/RimArcho/internal/app"

// The Steam client signed out by SteamCMD, and offline (VM run B11).
func init() {

	interfaceFormats = append(interfaceFormats, interfaceFormatFor(app.SteamSignOutNote), interfaceFormatFor("note: "+app.SteamSignOutNote))
}
