//go:build cgo

package gui

import (
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The Linux window (16 §8.2 "Linux"): GTK 3 and WebKitGTK 4.1 through Wails.

// webkitFloor is the oldest WebKitGTK the window runs on (16 §8.1): the page's CSS needs it.
const webkitFloor = "2.44"

// unavailable is why the window cannot open on this system; its text is complete, so the
// error box shows it as it is (cli.windowFailureText).
type unavailable struct{ text string }

func (e *unavailable) Error() string     { return e.text }
func (e *unavailable) Unavailable() bool { return true }

// platformApp sets what must be in place before Wails starts GTK.
func platformApp(_ *app.App, o *application.Options) {
	// WebKitGTK's DMA-BUF renderer draws a blank page on some drivers (VMware's SVGA, for one);
	// Wails turns it off only for NVIDIA. Off while the window is experimental; a player who
	// wants it sets WEBKIT_DISABLE_DMABUF_RENDERER=0.
	if os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") == "" {
		os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}
	// Wails' own handler quits the application from a signal, and WebKit's page process then
	// dies with SIGTRAP (a crash report); closeOnSignal closes the window instead.
	o.DisableDefaultSignalHandler = true
	// The window class and Wayland app id, whatever the program file is called (archo gui
	// in a cgo build): rimarcho.desktop's StartupWMClass matches it.
	o.Linux.ProgramName = "rimarcho"
}

// platformWindow sets the main window's Linux options. It starts hidden: fitWindow sizes it
// to the screen, then shows it.
func platformWindow(a *app.App, o *application.WebviewWindowOptions) {
	o.EnableFileDrop = true // Wails' GTK drop target; wireNativeDrop takes the files
	if cmds.Setting(a, "ui.gpu") == "off" {
		o.Linux.WebviewGpuPolicy = application.WebviewGpuPolicyNever // 16 §2.14
	}
	o.Hidden = true
}

// platformStart runs after the window is made and before Wails runs: it refuses an old
// WebKitGTK, fits the window to the screen once GTK is up, and closes it on a signal.
func platformStart(w *application.App, win *application.WebviewWindow) error {
	version, _ := w.Env.Info().PlatformInfo["webkit2gtk-runtime"].(string)
	w.Logger.Info("WebKitGTK", "version", version, "needed", webkitFloor)
	if version != "" && versionBelow(version, webkitFloor) {
		return &unavailable{view.Tf("RimArcho's window needs WebKitGTK %s or newer, and this system has %s.\n\nUpdate the system's WebKitGTK (the libwebkit2gtk-4.1-0 package on Ubuntu and Debian), or use archo in a terminal: the same manager as a text interface, which needs no WebKitGTK.", webkitFloor, version)}
	}
	shown := make(chan struct{})
	w.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		fitWindow(w, win)
		close(shown)
	})
	go func() {
		// not scheduled: shows the window if GTK's start event never came (a fail-safe, not polling)
		select {
		case <-shown:
		case <-time.After(5 * time.Second):
			win.Show()
		}
	}()
	closeOnSignal(w, win)
	return nil
}

// fitWindow sizes the window to at most 90% of the screen's work area, keeping the default
// 1440×880 where it fits, then centres and shows it. Windows does the same by itself.
func fitWindow(w *application.App, win *application.WebviewWindow) {
	defer win.Show()
	s, err := win.GetScreen()
	if err != nil || s == nil {
		if s = w.Screen.GetPrimary(); s == nil {
			return
		}
	}
	width, height := fitSize(1440, 880, s.WorkArea.Width, s.WorkArea.Height)
	win.SetSize(width, height)
	win.Center()
}

// fitSize is the window size for a work area: the default where it fits in 90% of the area,
// else 90% of it, never below the window's minimum (720×480).
func fitSize(width, height, areaW, areaH int) (int, int) {
	if areaW <= 0 || areaH <= 0 {
		return width, height
	}
	return max(720, min(width, areaW*9/10)), max(480, min(height, areaH*9/10))
}

// closeOnSignal: SIGTERM, SIGINT and SIGHUP close the window as its close button does
// (WindowClosing, so a running uninstall can still refuse); a second one quits.
func closeOnSignal(w *application.App, win *application.WebviewWindow) {
	ch := make(chan os.Signal, 3)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		<-ch
		win.Close()
		<-ch
		w.Quit()
	}()
}

// versionBelow compares dotted versions ("2.42.5" < "2.44").
func versionBelow(v, floor string) bool {
	a, b := strings.Split(v, "."), strings.Split(floor, ".")
	for i := range max(len(a), len(b)) {
		x, y := part(a, i), part(b, i)
		if x != y {
			return x < y
		}
	}
	return false
}

func part(p []string, i int) int {
	if i >= len(p) {
		return 0
	}
	n, _ := strconv.Atoi(p[i])
	return n
}
