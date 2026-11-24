package app

import (
	"context"
	"errors"
	"strings"
	"time"
)

// watchSteamPrompts watches a signed-in SteamCMD's console while it runs (11 §3 "Password
// prompt"): when its cursor line asks for the password or a Steam Guard code, the App's
// SteamCMDPrompter is asked and its answer typed into the console; a line asking to approve
// the sign-in in the Steam Mobile app is passed on once. shared: SteamCMD uses this
// process's console (the CLI), else its own (pid). stop ends the watch and returns a
// password or Guard refusal SteamCMD's own window showed, if any.
func (a *App) watchSteamPrompts(proc SteamCMDProc, shared bool) (stop func() *DownloadError) {
	p := a.SteamCMDPrompter
	var pid uint32
	if pp, ok := proc.(interface{ Pid() uint32 }); ok {
		pid = pp.Pid()
	}
	if p == nil || (!shared && pid == 0) {
		return func() *DownloadError { return nil }
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var refused *DownloadError
	go func() {
		defer close(done)
		poll := a.SteamCMDPoll
		if poll <= 0 || poll > 200*time.Millisecond {
			poll = 150 * time.Millisecond
		}
		tick := time.NewTicker(poll) // not scheduled: a process poll while SteamCMD's window is open
		defer tick.Stop()
		answered := map[string]bool{}
		phone := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			lines, y, err := steamConsoleRead(pid, shared)
			if errors.Is(err, errOwnConsole) {
				return // the player types in the window, as with no prompter
			}
			if err != nil || len(lines) == 0 {
				continue
			}
			if !shared { // a console of its own holds only this run
				login := steamLogin{}
				for _, l := range lines {
					login.read(l)
				}
				if login.err != nil && (login.err.Kind == "password" || login.err.Kind == "guard") {
					refused = login.err
				}
			}
			for _, l := range lines {
				if !phone && steamPhoneLine(l) {
					phone = true
					p.Phone()
				}
			}
			last := lines[len(lines)-1]
			kind := steamPromptKind(last)
			if kind == "" {
				continue
			}
			// One answer per prompt: the same prompt stays on screen until SteamCMD reads it.
			key := kind + "\x00" + strings.Join(lines[max(0, len(lines)-3):], "\n") + "\x00" + string(rune(y))
			if answered[key] {
				continue
			}
			answered[key] = true
			s := p.Secret(ctx, kind)
			if s.Empty() {
				continue
			}
			_ = steamConsoleType(pid, shared, s)
			s.Wipe()
		}
	}()
	return func() *DownloadError {
		cancel()
		<-done
		return refused
	}
}
