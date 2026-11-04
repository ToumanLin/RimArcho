package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/surface/upgradewin"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// upgradeShowAfter: an upgrade that ends sooner shows nothing.
const upgradeShowAfter = 300 * time.Millisecond

// upgradeReport shows a database upgrade while app.Open runs it (02 §7): after
// upgradeShowAfter, a small native window for RimArcho.exe (no console; window), else lines
// on stderr (CLI, MCP, and the TUI before it takes the terminal). lang: the text follows
// ui.lang (GUI, TUI); otherwise it is English, like the CLI.
type upgradeReport struct {
	window, lang bool
	dbPath       string
	stderr       io.Writer

	mu      sync.Mutex
	p       store.UpgradeProgress
	started time.Time
	stop    chan struct{}
	done    chan struct{}
	win     *upgradewin.Window
	printed bool // the stderr line is open
	tty     bool
}

// progress is app.OpenOptions.Upgrade.
func (u *upgradeReport) progress(p store.UpgradeProgress) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.p = p
	if u.stop == nil {
		u.started = time.Now()
		u.stop, u.done = make(chan struct{}), make(chan struct{})
		go u.run()
	}
}

func (u *upgradeReport) run() {
	defer close(u.done)
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-u.stop:
			return
		case <-t.C:
			u.mu.Lock()
			if time.Since(u.started) >= upgradeShowAfter {
				u.show()
			}
			u.mu.Unlock()
		}
	}
}

// show draws the current state (u.mu held).
func (u *upgradeReport) show() {
	if u.win == nil && !u.printed && u.lang {
		view.SetLang(store.PeekMeta(u.dbPath, "cfg:ui.lang"))
	}
	text := view.T("Upgrading the RimArcho database…")
	frac := -1.0
	if u.p.Total > 0 {
		frac = float64(u.p.Done) / float64(u.p.Total)
		text += fmt.Sprintf(" %d%%", int(frac*100))
	}
	switch {
	case u.window:
		if u.win == nil {
			u.win = upgradewin.Open("RimArcho", text)
		}
		u.win.Update(text, frac)
	case !u.printed:
		if fi, err := os.Stderr.Stat(); err == nil && u.stderr == os.Stderr {
			u.tty = fi.Mode()&os.ModeCharDevice != 0
		}
		u.printed = true
		if u.tty {
			fmt.Fprint(u.stderr, "\r"+text)
		} else {
			fmt.Fprintln(u.stderr, view.Tf("Upgrading the RimArcho database (schema %d → %d)…", u.p.From, u.p.To))
		}
	case u.tty:
		fmt.Fprint(u.stderr, "\r"+text)
	}
}

// finish ends the display after app.Open returned; ok: the upgrade succeeded.
func (u *upgradeReport) finish(ok bool) {
	u.mu.Lock()
	if u.stop == nil {
		u.mu.Unlock()
		return
	}
	close(u.stop)
	u.mu.Unlock()
	<-u.done
	u.mu.Lock()
	defer u.mu.Unlock()
	u.win.Close()
	if u.printed {
		took := time.Since(u.started).Seconds()
		msg := view.Tf("The database was upgraded in %.1f s.", took)
		if !ok {
			msg = view.T("The database upgrade did not finish.")
		}
		if u.tty {
			fmt.Fprint(u.stderr, "\r\x1b[K")
		}
		fmt.Fprintln(u.stderr, msg)
	}
}

// showOpenError is upgradewin.ShowError; tests replace it.
var showOpenError = upgradewin.ShowError

// openFailed reports an app.Open error in a message box when the program has no console
// (RimArcho.exe): otherwise the window would never appear and nothing would say why. The
// error is still written to stderr by the caller.
func (u *upgradeReport) openFailed(err error) {
	if !u.window {
		return
	}
	view.SetLang(store.PeekMeta(u.dbPath, "cfg:ui.lang"))
	text := view.Tf("RimArcho could not open its database:\n%s", err.Error())
	if errors.Is(err, store.ErrPreRelease) {
		text = view.Tf("This database was made by a RimArcho development build from before 1.0, and this version cannot open it:\n%s\n\nMove or rename the file, then start RimArcho again to begin with a new database.", u.dbPath)
	}
	showOpenError("RimArcho", text)
}
