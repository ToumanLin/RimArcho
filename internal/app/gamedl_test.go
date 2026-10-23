package app_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// The branches as SteamCMD listed them for an anonymous login on 2026-10-22
// (testdata/steamcmd): public, the public test branch, then the old versions, newest
// first; nothing guessed.
func TestParseBranches(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "testdata", "steamcmd", "app_info_print_294100.txt"))
	if err != nil {
		t.Fatal(err)
	}
	bs := app.ParseBranches(string(b))
	var names []string
	for _, x := range bs {
		names = append(names, x.Name)
	}
	want := []string{"public", "unstable", "version-1.6.4633", "version-1.5", "version-1.4.3901", "version-1.4.3563", "version-1.4.3542"}
	if len(names) < len(want) || !slices.Equal(names[:len(want)], want) {
		t.Fatalf("branches: %v", names)
	}
	if bs[2].Version != "1.6" || bs[2].Description != "Previous 1.6 version" || bs[2].BuildID != "20659247" || bs[2].Updated.IsZero() {
		t.Fatalf("version-1.6.4633: %+v", bs[2])
	}
	if !slices.Contains(names, "alpha4") || slices.Contains(names, "version-1.6") {
		t.Fatalf("only the branches Steam lists: %v", names)
	}
	if app.ParseBranches("Connecting anonymously to Steam Public...FAILED") != nil {
		t.Fatal("no app info, no branches")
	}
}

func steamZip(t *testing.T) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("steamcmd.exe")
	w.Write([]byte("MZ fake steamcmd"))
	zw.Close()
	return buf.Bytes()
}

// 08 §11: SteamCMD is downloaded from Valve only after the player saw the size, into
// %LOCALAPPDATA%\RimArcho\steamcmd.
func TestInstallSteamCMD(t *testing.T) {
	e := testkit.New(t)
	z := steamZip(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(z)))
		if r.Method != http.MethodHead {
			w.Write(z)
		}
	}))
	defer srv.Close()
	e.App.SteamCMDZip = srv.URL + "/steamcmd.zip"
	e.App.SteamCMDNoPath = true
	if p := e.App.SteamCMDPath(); p != "" {
		t.Fatalf("no SteamCMD yet: %s", p)
	}
	if _, err := e.App.GameBranches(); !errors.Is(err, app.ErrNoSteamCMD) {
		t.Fatalf("branches without SteamCMD: %v", err)
	}
	o, err := e.App.OfferSteamCMD()
	if err != nil || o.Bytes != int64(len(z)) || !strings.HasSuffix(filepath.ToSlash(o.Dir), "LocalAppData/RimArcho/steamcmd") {
		t.Fatalf("offer: %+v %v", o, err)
	}
	exe, err := e.App.InstallSteamCMD()
	if err != nil {
		t.Fatal(err)
	}
	if e.App.SteamCMDPath() != exe || filepath.Dir(exe) != o.Dir {
		t.Fatalf("installed at %s, found %s", exe, e.App.SteamCMDPath())
	}
}

// fakeSteamCMD installs a stand-in steamcmd and a start function that writes log lines (to
// logs/console_log.txt, as SteamCMD does) and, on success, a RimWorld into the folder.
type fakeSteamCMD struct {
	args  []string
	lines []string
	game  bool // write a game into +force_install_dir
}

func (f *fakeSteamCMD) install(t *testing.T, e *testkit.Env) {
	dir := e.App.SteamCMDDir()
	e.WriteFile(filepath.Join(dir, "steamcmd.exe"), "fake")
	e.WriteFile(filepath.Join(dir, "logs", "console_log.txt"), "[2026-10-21 10:00:00] an older run\n")
	e.App.SteamCMDNoPath = true
	e.App.SteamCMDPoll = time.Millisecond
	e.App.SteamCMDStart = func(exe string, args []string, console bool) (app.SteamCMDProc, error) {
		f.args = args
		return fakeSteamCMDProc{f: f, e: e, dir: dir, args: args}, nil
	}
}

type fakeSteamCMDProc struct {
	f    *fakeSteamCMD
	e    *testkit.Env
	dir  string
	args []string
}

func (p fakeSteamCMDProc) Wait() error {
	lf, err := os.OpenFile(filepath.Join(p.dir, "logs", "console_log.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer lf.Close()
	for _, l := range p.f.lines {
		lf.WriteString("[2026-10-22 01:00:00] " + l + "\n")
		time.Sleep(2 * time.Millisecond)
	}
	if p.f.game {
		for i, a := range p.args {
			if a == "+force_install_dir" {
				g := p.args[i+1]
				p.e.WriteFile(filepath.Join(g, "RimWorldWin64.exe"), "")
				p.e.WriteFile(filepath.Join(g, "Version.txt"), "1.5.4409 rev1118\n")
				p.e.MkdirAll(filepath.Join(g, "Mods"))
				p.e.CopyTemplate("core", filepath.Join(g, "Data", "Core"))
			}
		}
	}
	return nil
}

var okRun = []string{
	"Logging in user 'player' [U:1:12345] to Steam Public...OK",
	"Waiting for user info...OK",
	" Update state (0x3) reconfiguring, progress: 0.00 (0 / 0)",
	" Update state (0x61) downloading, progress: 12.50 (50000000 / 400000000)",
	" Update state (0x61) downloading, progress: 75.00 (300000000 / 400000000)",
	" Update state (0x81) verifying update, progress: 50.00 (200000000 / 400000000)",
	"Success! App '294100' fully installed.",
}

// 08 §11: a version downloaded with SteamCMD becomes an environment named after its
// version; only the account name reaches SteamCMD; progress comes from SteamCMD's log.
func TestDownloadGame(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	f := &fakeSteamCMD{lines: okRun, game: true}
	f.install(t, e)
	dir := filepath.Join(e.Root, "Games", "RimWorld15")
	var states []string
	var pct []float64
	r, err := e.App.DownloadGame(app.DownloadSpec{Branch: "version-1.5", Dir: dir, Account: "player"}, func(p app.DownloadProgress) {
		if len(states) == 0 || states[len(states)-1] != p.State {
			states = append(states, p.State)
		}
		if p.State == "downloading" {
			pct = append(pct, p.Percent)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"+force_install_dir", dir, "+login", "player", "+app_update", "294100", "-beta", "version-1.5", "validate", "+quit"}
	if !slices.Equal(f.args, want) {
		t.Fatalf("args: %v", f.args)
	}
	if !slices.Equal(states, []string{"starting", "login", "downloading", "verifying", "done"}) || !slices.Equal(pct, []float64{0, 12.5, 75}) {
		t.Fatalf("progress: %v %v", states, pct)
	}
	if r.Env.Name != "1.5" || r.Env.GameRoot != dir || r.Env.Launch != "direct" {
		t.Fatalf("env: %+v", r.Env)
	}
	// The public branch has no -beta; a second 1.5 gets a free name.
	f.args = nil
	f.lines = append([]string{"Connecting anonymously to Steam Public...OK"}, okRun[2:]...)
	r, err = e.App.DownloadGame(app.DownloadSpec{Branch: "public", Dir: filepath.Join(e.Root, "Games", "Again")}, nil)
	if err != nil || slices.Contains(f.args, "-beta") || f.args[3] != "anonymous" || r.Env.Name != "1.5-2" {
		t.Fatalf("public: %v %v %+v", err, f.args, r)
	}
}

// 08 §11: failures in plain words; the folder must not be RimArcho's or inside a source.
func TestDownloadGameErrors(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	f := &fakeSteamCMD{}
	f.install(t, e)
	dir := filepath.Join(e.Root, "Games", "X")
	for _, c := range []struct {
		lines []string
		kind  string
		text  string
	}{
		{[]string{"Logging in user 'player' [U:1:0] to Steam Public...FAILED (Invalid Password)"}, "password", "did not accept the password"},
		{[]string{"Logging in user 'player' [U:1:0] to Steam Public...FAILED (Two-factor code mismatch)"}, "guard", "Steam Guard"},
		{[]string{okRun[0], "ERROR! Failed to install app '294100' (No subscription)"}, "ownership", "does not own RimWorld"},
		{[]string{okRun[0], okRun[3], "ERROR! Failed to install app '294100' (Disk Write Failure)"}, "disk", "disk is full"},
		{[]string{okRun[0], "ERROR! Failed to install app '294100' (Invalid Beta)"}, "branch", "does not have this branch"},
		{[]string{"Redirecting stderr"}, "login", "login did not finish"},
	} {
		f.lines = c.lines
		_, err := e.App.DownloadGame(app.DownloadSpec{Branch: "version-1.5", Dir: dir, Account: "player"}, nil)
		var de *app.DownloadError
		if !errors.As(err, &de) || de.Kind != c.kind || !strings.Contains(err.Error(), c.text) {
			t.Errorf("%v: %v", c.lines, err)
		}
	}
	for _, bad := range []string{filepath.Join(e.Home, "game"), filepath.Join(e.Workshop, "game"), e.Game, filepath.Join(e.Game, "Mods", "x")} {
		if _, err := e.App.DownloadGame(app.DownloadSpec{Dir: bad, Account: "player"}, nil); err == nil {
			t.Errorf("%s must be refused", bad)
		}
	}
	if _, err := e.App.DownloadGame(app.DownloadSpec{Dir: dir, Account: "a b"}, nil); err == nil {
		t.Error("a name with a space is not an account name")
	}
}
