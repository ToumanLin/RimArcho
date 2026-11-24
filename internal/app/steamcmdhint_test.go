package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// signIns records App.SteamCMDSignIn calls, and the starts of SteamCMD between them.
type signIns struct {
	mu     sync.Mutex
	events []string
}

func (s *signIns) add(e string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
}

func (s *signIns) get() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.events)
}

func (s *signIns) hook(open bool) {
	if open {
		s.add("open")
	} else {
		s.add("close")
	}
}

// 11 §3 "Password prompt" (#19): the surface is told before a signed-in SteamCMD run starts
// and after it ends (a game download: once the download begins); anonymous runs say nothing.
func TestSteamPasswordHints(t *testing.T) {
	t.Parallel()
	for account, want := range map[string]bool{"": false, "anonymous": false, " Anonymous ": false, "player": true} {
		if got := app.SteamCMDSignsIn(account); got != want {
			t.Errorf("SteamCMDSignsIn(%q) = %v", account, got)
		}
	}
	if len(app.SteamCMDPasswordHints) != 3 || !strings.Contains(app.SteamPasswordRefused, "did not accept the password") {
		t.Fatal("the hints changed shape")
	}
	e := testkit.New(t)
	e.AddEnv(false)
	f := &fakeSteamCMD{lines: okRun, game: true}
	f.install(t, e)
	rec := &signIns{}
	e.App.SteamCMDSignIn = rec.hook
	start := e.App.SteamCMDStart
	e.App.SteamCMDStart = func(exe string, args []string, console bool) (app.SteamCMDProc, error) {
		rec.add("start")
		return start(exe, args, console)
	}
	var atDownload []string
	_, err := e.App.DownloadGame(app.DownloadSpec{Branch: "version-1.5", Dir: filepath.Join(e.Root, "Games", "A"), Account: "player"}, func(p app.DownloadProgress) {
		if p.State == "downloading" && atDownload == nil {
			atDownload = rec.get()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.get(); !slices.Equal(got, []string{"open", "start", "close"}) || !slices.Equal(atDownload, got) {
		t.Fatalf("signed-in download: %v (when the download began: %v)", got, atDownload)
	}
	rec.events = nil
	f.lines = append([]string{"Connecting anonymously to Steam Public...OK"}, okRun[1:]...)
	_, _ = e.App.DownloadGame(app.DownloadSpec{Branch: "version-1.5", Dir: filepath.Join(e.Root, "Games", "B"), Account: "anonymous"}, nil)
	if got := rec.get(); slices.Contains(got, "open") {
		t.Fatalf("anonymous download told the surface: %v", got)
	}
}

// #19: a password the sign-in window's SteamCMD refused is named, with the hints, instead
// of "login required"; the surface is told around the window only.
func TestSteamLoginReportsRefusedPassword(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, window, want string
	}{
		{"password", "Logging in user 'dummyPlayer' [U:1:0] to Steam Public...FAILED (Invalid Password)\n", app.SteamPasswordRefused},
		{"guard", "Logging in user 'dummyPlayer' [U:1:0] to Steam Public...FAILED (Two-factor code mismatch)\n", "Steam did not accept the Steam Guard code"},
		{"closed", "Logging in user 'dummyPlayer' to Steam Public...\n", "login required; sign in again to complete password and Steam Guard verification"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := testkit.New(t)
			e.App.SteamCMDNoPath = true
			dir := filepath.Join(e.App.LocalDir(), "steamcmd")
			e.WriteFile(filepath.Join(dir, deps.SteamCMDExecutable()), "offline fixture")
			e.MustRun("config", "set", "steamcmd.account", "dummyPlayer")
			log := filepath.Join(dir, "logs", "console_log.txt")
			e.WriteFile(log, "an earlier run: FAILED (Invalid Password)\n")
			rec := &signIns{}
			e.App.SteamCMDSignIn = rec.hook
			e.App.SteamCMDStart = func(_ string, args []string, console bool) (app.SteamCMDProc, error) {
				rec.add("start")
				text := "Cached credentials not found.\n"
				if console {
					text = tc.window
				}
				return accountProc{func() error {
					f, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0o600)
					if err != nil {
						return err
					}
					defer f.Close()
					_, err = f.WriteString(text)
					return err
				}}, nil
			}
			err := e.App.SteamAccountAction("login")
			if err == nil || err.Error() != tc.want {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			var de *app.DownloadError
			if (tc.name != "closed") != errors.As(err, &de) {
				t.Fatalf("error type: %T", err)
			}
			if got := rec.get(); !slices.Equal(got, []string{"open", "start", "close", "start"}) {
				t.Fatalf("surface told: %v", got)
			}
			if e.App.SteamAccountStatus().LoggedIn {
				t.Fatal("signed in after a refusal")
			}
		})
	}
}
