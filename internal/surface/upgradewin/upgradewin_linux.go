// Package upgradewin shows a small native window while the database upgrades,
// before the GUI window opens (04 §4, 16 §8.1).
package upgradewin

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Window is zenity's progress dialog on Linux (16 §8.1), or nil without zenity or a
// desktop. Its methods are safe on a nil receiver.
type Window struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	in  io.WriteCloser
}

// desktop reports whether a dialog can show: GTK needs a display.
func desktop() bool { return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" }

// Open shows the progress dialog, or returns nil when zenity or the desktop is missing.
func Open(title, text string) *Window {
	zenity, err := exec.LookPath("zenity")
	if err != nil || !desktop() {
		return nil
	}
	c := exec.Command(zenity, "--progress", "--title="+title, "--text="+text, "--no-cancel", "--auto-close", "--width=380")
	in, err := c.StdinPipe()
	if err != nil || c.Start() != nil {
		return nil
	}
	return &Window{cmd: c, in: in}
}

// Update sets the text and, for frac ≥ 0, the bar (zenity reads "# text" and percent lines).
func (w *Window) Update(text string, frac float64) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.in == nil {
		return
	}
	fmt.Fprintf(w.in, "# %s\n", text)
	if frac >= 0 {
		fmt.Fprintf(w.in, "%d\n", min(99, int(frac*100))) // 100 would close it
	}
}

// Close ends the dialog: at 100 % zenity closes itself (--auto-close); one that has not
// within two seconds is killed. Closing twice is safe.
func (w *Window) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.in == nil {
		return
	}
	fmt.Fprintln(w.in, "100")
	w.in.Close()
	w.in = nil
	done := make(chan struct{})
	go func() { w.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second): // not scheduled: a bound on waiting for zenity to exit
		w.cmd.Process.Kill()
		<-done
	}
}
