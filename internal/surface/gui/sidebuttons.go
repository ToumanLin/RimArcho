//go:build windows || cgo

package gui

// GDK 3 event types and the X11 numbers of the mouse side buttons (16 §3.1 rule 11).
// Wayland's BTN_SIDE and BTN_EXTRA reach GTK as the same 8 and 9.
const (
	gdkButtonPress   = 4 // GDK_BUTTON_PRESS
	gdkButtonBack    = 8
	gdkButtonForward = 9
)

// sideStep reads a GTK button event on the Linux window's web view (sidebuttons_linux.go).
// Buttons 8 and 9 are always swallowed, so WebKitGTK never hands them to the page, where
// they arrive as `button` 0 (a left button to the page's own handlers) with no way to tell
// back from forward. Only the first press moves: GTK follows a quick second press with a
// GDK_2BUTTON_PRESS, which must not take another step. Other buttons pass through.
func sideStep(eventType, button uint32) (step int, swallow bool) {
	switch button {
	case gdkButtonBack:
		step = -1
	case gdkButtonForward:
		step = 1
	default:
		return 0, false
	}
	if eventType != gdkButtonPress {
		step = 0
	}
	return step, true
}
