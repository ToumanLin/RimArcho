package upgradewin

import "os/exec"

// ShowError shows a startup error on the desktop and waits until it is closed (16 §8.1):
// zenity (GNOME and most others), else kdialog (KDE), else a desktop notification. Without
// any of them, or without a desktop, the caller's stderr line is all there is.
func ShowError(title, text string) {
	if !desktop() {
		return
	}
	if p, err := exec.LookPath("zenity"); err == nil {
		exec.Command(p, "--error", "--title="+title, "--text="+text, "--no-markup", "--width=480").Run()
		return
	}
	if p, err := exec.LookPath("kdialog"); err == nil {
		exec.Command(p, "--title", title, "--error", text).Run()
		return
	}
	if p, err := exec.LookPath("notify-send"); err == nil {
		exec.Command(p, "--urgency=critical", "--app-name=RimArcho", title, text).Run()
	}
}
