package testkit

import (
	"runtime"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/textures"
)

// The golden transcripts are recorded on Windows. These lines differ on purpose on
// other systems; WindowsShaped maps them back so one transcript serves both.
var windowsShape = []struct{ other, windows string }{
	// Linux's SteamCMD launcher is steamcmd.sh.
	{"/steamcmd.sh", "/steamcmd.exe"},
	// Linux downloads no todds (Linux support plan, step 12), so its help and errors say so.
	{"download steamcmd into RimArcho's own folder (provide todds separately)", "download todds or steamcmd into RimArcho's own folder"},
	{"locate the native DDS encoder (todds) on PATH or at textures.todds", "the DDS encoder (todds): where it is, or --install to download it"},
	// `archo deps` reports a missing installable dependency generically on Windows.
	{"    error: " + textures.ToddsMissing, "    error: todds is not installed"},
	{textures.ToddsMissing, "todds (the DDS encoder) is not installed; archo textures tool --install downloads it"},
}

// WindowsShaped returns out as the Windows build would print it.
func WindowsShaped(out string) string {
	if runtime.GOOS == "windows" {
		return out
	}
	for _, r := range windowsShape {
		out = strings.ReplaceAll(out, r.other, r.windows)
	}
	return out
}
