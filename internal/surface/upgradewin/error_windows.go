package upgradewin

import "golang.org/x/sys/windows"

// ShowError shows a modal error message box and returns when the player closes it. It is
// for failures before the GUI window exists, which RimArcho.exe (no console) could
// otherwise report only on stderr (02 §7).
func ShowError(title, text string) {
	t, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	c, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	windows.MessageBox(0, t, c, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}
