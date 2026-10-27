package app

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/vdf"
)

// Downloading a game version with SteamCMD (01 §3, 08 §11). Anonymous by default; an
// optional account name comes from settings or the download form. Passwords and Steam
// Guard codes are entered only in SteamCMD's own console. Progress comes from its log.

// SteamCMDURL is Valve's SteamCMD for Windows.
const SteamCMDURL = "https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip"

// SteamCMDProc is a running SteamCMD.
type SteamCMDProc interface{ Wait() error }

// steamCMDRunner starts SteamCMD; console: in a console window of its own (where the
// player logs in), else hidden with its output returned by run.
type steamCMDRunner struct {
	start func(exe string, args []string, console bool) (SteamCMDProc, error)
	run   func(exe string, args []string) (string, error)
}

// SteamCMDDir is where RimArcho keeps the SteamCMD it downloads: %LOCALAPPDATA%\RimArcho\steamcmd.
func (a *App) SteamCMDDir() string {
	if p := a.Store.Meta("cfg:steamcmd.path"); p != "" {
		return filepath.Dir(p)
	}
	return filepath.Join(a.LocalDir(), "steamcmd")
}

// SteamCMDPath finds steamcmd: config steamcmd.path, RimArcho's own copy, then PATH. ""
// when there is none.
func (a *App) SteamCMDPath() string {
	return a.Deps().Status(deps.SteamCMD).Path
}

// SteamCMDOffer is what downloading SteamCMD would do: from where, to where, how big.
type SteamCMDOffer struct {
	URL   string `json:"url"`
	Dir   string `json:"dir"`
	Bytes int64  `json:"bytes"` // -1 when the server did not say
}

func (a *App) steamCMDURL() string {
	if a.SteamCMDZip != "" {
		return a.SteamCMDZip
	}
	return SteamCMDURL
}

// OfferSteamCMD asks Valve's server for the size of SteamCMD (a HEAD request), so the player
// can be asked first.
func (a *App) OfferSteamCMD() (*SteamCMDOffer, error) {
	o := &SteamCMDOffer{URL: a.steamCMDURL(), Dir: a.SteamCMDDir(), Bytes: -1}
	req, err := http.NewRequest(http.MethodHead, o.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return o, nil // the size stays unknown; the download itself reports a failure
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK && resp.ContentLength > 0 {
		o.Bytes = resp.ContentLength
	}
	return o, nil
}

// InstallSteamCMD downloads SteamCMD from Valve and unpacks it into SteamCMDDir. It runs
// only when the player said yes to OfferSteamCMD. SteamCMD updates itself on its first run.
func (a *App) InstallSteamCMD() (string, error) {
	return a.InstallSteamCMDProgress(nil)
}

// InstallSteamCMDProgress also reports bytes received to the guide and task (06 §2.15).
func (a *App) InstallSteamCMDProgress(progress func(done, total int64)) (string, error) {
	return a.installSteamCMDProgress(a.SteamCMDDir(), progress)
}

func (a *App) installSteamCMDProgress(dir string, progress func(done, total int64)) (string, error) {
	unlock := accounts.Lock(filepath.Join(dir, "steamcmd.exe"))
	defer unlock()
	if err := a.checkRoots(dir); err != nil {
		return "", err
	}
	c := a.httpClient()
	if c.Timeout != 0 && c.Timeout < 2*time.Minute {
		cc := *c
		cc.Timeout = 2 * time.Minute
		c = &cc
	}
	resp, err := c.Get(a.steamCMDURL())
	if err != nil {
		return "", fmt.Errorf("downloading SteamCMD: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading SteamCMD: %s", resp.Status)
	}
	b, err := readToolDownload(resp, progress)
	if err != nil {
		return "", fmt.Errorf("downloading SteamCMD: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("SteamCMD download is not a zip: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	exe := ""
	for _, f := range zr.File {
		name := filepath.Base(f.Name)
		if f.FileInfo().IsDir() || name != f.Name || strings.Contains(f.Name, "..") {
			continue // flat zip expected; nothing outside dir
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		p := filepath.Join(dir, name)
		w, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err == nil {
			_, err = io.Copy(w, rc)
			if cerr := w.Close(); err == nil {
				err = cerr
			}
		}
		rc.Close()
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(strings.ToLower(name), "steamcmd") {
			exe = p
		}
	}
	if exe == "" {
		return "", errors.New("the SteamCMD download has no steamcmd program")
	}
	if err := a.Store.SetMeta(directoryKey("steamcmd-tool"), dir); err != nil {
		return "", err
	}
	return exe, nil
}

// ErrNoSteamCMD: SteamCMD is not on this computer (OfferSteamCMD, InstallSteamCMD).
var ErrNoSteamCMD = errors.New("SteamCMD is not installed")

// GameBranch is one Steam branch of RimWorld (app 294100), as app_info_print lists it.
type GameBranch struct {
	Name        string    `json:"name"`        // "public", "version-1.5", …
	Description string    `json:"description"` // Steam's, e.g. "Previous 1.6 version"
	Version     string    `json:"version"`     // major.minor read from the name ("" for public / unstable)
	BuildID     string    `json:"buildId"`
	Updated     time.Time `json:"updated"`
}

var branchVersionRe = regexp.MustCompile(`^version-(\d+\.\d+)`)

// GameBranches returns public RimWorld branches, reusing the process cache.
func (a *App) GameBranches() ([]GameBranch, error) {
	exe := a.SteamCMDPath()
	bs, _, err := a.GameBranchesCached(exe)
	if len(bs) > 0 {
		return bs, nil
	}
	return bs, err
}

// GameBranchesCached uses an already resolved SteamCMD path. Refreshing is true
// while stale branches are being refreshed; callers can read again on completion.
// A failed refresh returns the old branches with the error.
func (a *App) GameBranchesCached(exe string) ([]GameBranch, bool, error) {
	return a.GameBranchesCachedContext(context.Background(), exe)
}

// GameBranchesCachedContext ties branch queries and stale refreshes to the caller's
// lifetime. Downloads keep their own runner and process lifecycle.
func (a *App) GameBranchesCachedContext(ctx context.Context, exe string) ([]GameBranch, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if exe == "" {
		return nil, false, ErrNoSteamCMD
	}
	key, err := branchToolKey(exe, env.WorkshopAppID)
	if err != nil {
		return nil, false, err
	}
	run := a.SteamCMDRun
	return gameBranchesCache.get(key, time.Now, func() ([]GameBranch, error) {
		unlock := accounts.Lock(exe)
		defer unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		args := []string{"+login", "anonymous", "+app_info_update", "1", "+app_info_print", env.WorkshopAppID, "+quit"}
		var out string
		var err error
		if run != nil {
			out, err = run(exe, args)
		} else {
			out, err = runBranchSteamCMD(ctx, exe, args)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, fmt.Errorf("SteamCMD could not read RimWorld's branches: %v", err)
		}
		bs := ParseBranches(out)
		if len(bs) == 0 {
			return nil, errors.New("SteamCMD did not list RimWorld's branches (is Steam reachable?)")
		}
		return bs, nil
	}, func() (branchKey, error) { return branchToolKey(exe, env.WorkshopAppID) })
}

// ParseBranches reads the branches from app_info_print's output: public first, the public
// test branch, then the old versions newest first.
func ParseBranches(out string) []GameBranch {
	i := strings.Index(out, `"`+env.WorkshopAppID+`"`)
	if i < 0 {
		return nil
	}
	body := out[i:]
	// The VDF block ends where SteamCMD's own lines start again.
	depth, end := 0, -1
	for j, c := range body {
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = j + 1
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil
	}
	root, err := vdf.Parse([]byte(body[:end]))
	if err != nil {
		return nil
	}
	app := root.Get(env.WorkshopAppID)
	if app == nil {
		app = root
	}
	br := app.Get("depots").Get("branches")
	if br == nil {
		return nil
	}
	var out2 []GameBranch
	for _, b := range br.Children {
		if b.Children == nil || b.Str("pwdrequired") == "1" {
			continue
		}
		g := GameBranch{Name: b.Key, Description: b.Str("description"), BuildID: b.Str("buildid")}
		if t, err := strconv.ParseInt(b.Str("timeupdated"), 10, 64); err == nil && t > 0 {
			g.Updated = time.Unix(t, 0).UTC()
		}
		if m := branchVersionRe.FindStringSubmatch(b.Key); m != nil {
			g.Version = m[1]
		}
		out2 = append(out2, g)
	}
	rank := func(g GameBranch) int {
		switch {
		case g.Name == "public":
			return 0
		case g.Name == "unstable":
			return 1
		case g.Version != "":
			return 2
		}
		return 3 // alpha / beta of before 1.0
	}
	sort.SliceStable(out2, func(i, j int) bool {
		a, b := out2[i], out2[j]
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if a.Version != b.Version {
			return env.VersionLess(b.Version, a.Version)
		}
		return a.Updated.After(b.Updated)
	})
	return out2
}

// DownloadSpec is a game version to download.
type DownloadSpec struct {
	Branch  string // "public" or a branch from GameBranches
	Dir     string // the new game folder
	Account string // override; empty uses settings, then anonymous
	Name    string // the environment's name ("" = the version, e.g. "1.6")
	Saves   string // SavesShared or SavesOwn ("" = shared)
	// Console: SteamCMD gets a console window of its own for the login (the window);
	// false runs it in the caller's console (the CLI).
	Console bool
}

// DownloadProgress is what SteamCMD's log says so far.
type DownloadProgress struct {
	State   string  `json:"state"`   // starting | login | downloading | verifying | done
	Percent float64 `json:"percent"` // of the current state
	Done    int64   `json:"done"`
	Total   int64   `json:"total"`
	Line    string  `json:"line"` // the last line read, as SteamCMD wrote it
}

// DownloadError is a failed download in plain words.
type DownloadError struct {
	Kind string // ownership | anonymous-license | anonymous-login | password | guard | permission | disk | branch | network | login | other
	Line string // SteamCMD's line
}

func (e *DownloadError) Error() string {
	return e.Why()
}

// Why is the reason alone, in plain words.
func (e *DownloadError) Why() string {
	var why string
	switch e.Kind {
	case "anonymous-license":
		why = "anonymous accounts cannot download RimWorld; log in with an account that owns it, or add a game folder you already have"
	case "anonymous-login":
		why = "the anonymous Steam connection did not finish; check the connection and try again"
	case "ownership":
		why = "this Steam account does not own RimWorld"
	case "password":
		why = "Steam did not accept the password"
	case "guard":
		why = "Steam did not accept the Steam Guard code"
	case "disk":
		why = "the disk is full (or the folder cannot be written)"
	case "branch":
		why = "Steam does not have this branch (any more)"
	case "network":
		why = "Steam could not be reached"
	case "login":
		why = "the login did not finish (the SteamCMD window was closed?)"
	default:
		why = "SteamCMD did not finish the download"
	}
	return why
}

var accountRe = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,64}$`)

// ValidSteamCMDAccount accepts a name or empty (anonymous); never a password or command.
func ValidSteamCMDAccount(account string) error {
	if account != "" && !accountRe.MatchString(account) {
		return errors.New("the SteamCMD account must be empty (anonymous), or a name using letters, digits and _ . @ -")
	}
	return nil
}

// SteamCMDAccount resolves a per-run override, then settings, then anonymous. Public
// metadata queries keep using anonymous even when the player has configured an account.
func (a *App) SteamCMDAccount(override string) (string, error) {
	account := strings.TrimSpace(override)
	if account == "" {
		account = strings.TrimSpace(a.Store.Meta("cfg:steamcmd.account"))
	}
	if account == "" || strings.EqualFold(account, "anonymous") {
		account = "anonymous"
	}
	if err := ValidSteamCMDAccount(account); err != nil {
		return "", err
	}
	return account, nil
}

// CheckDownloadDir: the new game folder may not be inside RimArcho's data directory, nor
// inside or around any source (another game, a mod folder).
func (a *App) CheckDownloadDir(dir string) (string, error) {
	dir = strings.Trim(strings.TrimSpace(dir), `"'`)
	if dir == "" {
		return "", errors.New("choose the folder for the game")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if source.Within(dir, a.Home) || source.Within(a.Home, dir) {
		return "", fmt.Errorf("%s is inside RimArcho's data folder (or holds it); choose a folder of its own", dir)
	}
	if o, err := a.overlapping(dir, nil); err != nil {
		return "", err
	} else if o != nil {
		return "", fmt.Errorf("%s overlaps src:%s (%s); choose a folder of its own", dir, o.Label, o.Root)
	}
	if e, err := a.envOfGame(dir); err != nil {
		return "", err
	} else if e != nil {
		return "", fmt.Errorf("environment %s already uses %s", e.Name, dir)
	}
	return dir, nil
}

// DownloadGame runs SteamCMD (+force_install_dir <dir> +login <account> +app_update 294100
// [-beta <branch>] validate +quit) and, when it succeeds, adds the folder as an environment,
// as a game the player already has. progress is called with each step SteamCMD's log shows.
func (a *App) DownloadGame(spec DownloadSpec, progress func(DownloadProgress)) (*SourceChangeEnv, error) {
	dir, err := a.FetchGame(spec, progress)
	if err != nil {
		return nil, err
	}
	return a.AddDownloaded(spec, dir)
}

// FetchGame is DownloadGame's SteamCMD run: it returns the game folder. It only reads
// RimArcho's database, so the window runs it without its lock.
func (a *App) FetchGame(spec DownloadSpec, progress func(DownloadProgress)) (string, error) {
	exe := a.SteamCMDPath()
	if exe == "" {
		return "", ErrNoSteamCMD
	}
	unlock := accounts.Lock(exe)
	defer unlock()
	account, err := a.SteamCMDAccount(spec.Account)
	if err != nil {
		return "", err
	}
	if account == "anonymous" {
		return "", &DownloadError{Kind: "anonymous-license"}
	}

	if spec.Branch == "" {
		spec.Branch = "public"
	}
	if !regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`).MatchString(spec.Branch) {
		return "", fmt.Errorf("%q is not a Steam branch name", spec.Branch)
	}
	dir, err := a.CheckDownloadDir(spec.Dir)
	if err != nil {
		return "", err
	}
	if spec.Name != "" {
		if err := ValidEnvName(spec.Name); err != nil {
			return "", err
		}
		if e, _ := a.Store.EnvByName(spec.Name); e != nil {
			return "", fmt.Errorf("environment %q already exists", spec.Name)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	args := []string{"+force_install_dir", dir, "+login", account, "+app_update", env.WorkshopAppID}
	if spec.Branch != "public" {
		args = append(args, "-beta", spec.Branch)
	}
	args = append(args, "validate", "+quit")
	logPath := filepath.Join(filepath.Dir(exe), "logs", "console_log.txt")
	var off int64
	if st, err := os.Stat(logPath); err == nil {
		off = st.Size()
	}
	if progress == nil {
		progress = func(DownloadProgress) {}
	}
	progress(DownloadProgress{State: "starting"})
	proc, err := a.steamCMD().start(exe, args, spec.Console)
	if err != nil {
		return "", fmt.Errorf("starting SteamCMD: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- proc.Wait() }()
	t := &logTail{path: logPath, off: off}
	var res steamResult
	tick := time.NewTicker(a.steamPoll())
	defer tick.Stop()
	var werr error
	for running := true; running; {
		select {
		case werr = <-done:
			running = false
		case <-tick.C:
		}
		for _, l := range t.lines() {
			if p, ok := res.read(l); ok {
				p.Line = p.State
				progress(p)
			}
		}
	}
	if res.err != nil {
		if account == "anonymous" {
			switch res.err.Kind {
			case "ownership":
				res.err.Kind = "anonymous-license"
			case "login":
				res.err.Kind = "anonymous-login"
			}
		}
		return "", res.err
	}
	if !res.success {
		line := res.last
		if werr != nil && line == "" {
			line = werr.Error()
		}
		if !res.loggedIn {
			kind := "login"
			if account == "anonymous" {
				kind = "anonymous-login"
			}
			return "", &DownloadError{Kind: kind, Line: line}
		}
		return "", &DownloadError{Kind: "other", Line: line}
	}
	if !env.LooksLikeGameRoot(dir) {
		return "", fmt.Errorf("SteamCMD says it finished, but %s has no RimWorld", dir)
	}
	progress(DownloadProgress{State: "done", Percent: 100})
	return dir, nil
}

// AddDownloaded adds a downloaded game folder as an environment named spec.Name, else after
// its version ("1.6").
func (a *App) AddDownloaded(spec DownloadSpec, dir string) (*SourceChangeEnv, error) {
	name := spec.Name
	if name == "" {
		name = a.freeEnvName(env.MajorMinor(env.ReadGameVersion(dir)))
	}
	e, changes, err := a.EnvAdd(name, EnvSpec{GameRoot: dir, Launch: "direct", Saves: spec.Saves})
	if err != nil {
		return nil, err
	}
	return &SourceChangeEnv{Env: e, Changes: changes}, nil
}

// SourceChangeEnv is an environment just added with what happened to its sources.
type SourceChangeEnv struct {
	Env     *env.Environment
	Changes []SourceChange
}

// freeEnvName is base ("1.6"), or base-2… when an environment has it.
func (a *App) freeEnvName(base string) string {
	if base == "" {
		base = "rimworld"
	}
	for i := 1; ; i++ {
		n := base
		if i > 1 {
			n = fmt.Sprintf("%s-%d", base, i)
		}
		if e, _ := a.Store.EnvByName(n); e == nil {
			return n
		}
	}
}

func (a *App) steamPoll() time.Duration {
	if a.SteamCMDPoll > 0 {
		return a.SteamCMDPoll
	}
	return 500 * time.Millisecond
}

func (a *App) steamCMD() steamCMDRunner {
	r := steamCMDRunner{start: startSteamCMD, run: runSteamCMD}
	if a.SteamCMDStart != nil {
		r.start = a.SteamCMDStart
	}
	if a.SteamCMDRun != nil {
		r.run = a.SteamCMDRun
	}
	return r
}

func runSteamCMD(exe string, args []string) (string, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func startSteamCMD(exe string, args []string, console bool) (SteamCMDProc, error) {
	if console {
		return startConsole(exe, args)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// logTail reads the lines appended to SteamCMD's console log since off.
type logTail struct {
	path    string
	off     int64
	partial string
}

func (t *logTail) lines() []string {
	f, err := os.Open(t.path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() < t.off {
		t.off, t.partial = 0, "" // SteamCMD started a new log
	}
	if _, err := f.Seek(t.off, io.SeekStart); err != nil {
		return nil
	}
	b, _ := io.ReadAll(f)
	t.off += int64(len(b))
	s := t.partial + string(b)
	var out []string
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	t.partial = ""
	if len(s) > 0 && s[len(s)-1] != '\n' && len(out) > 0 {
		t.partial, out = out[len(out)-1], out[:len(out)-1]
	}
	return out
}

// steamResult is what the log said: success, a known failure, or the last line.
type steamResult struct {
	login             steamLogin
	success, loggedIn bool
	err               *DownloadError
	last              string
}

var (
	stampRe    = regexp.MustCompile(`^\[\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\]\s?`)
	progressRe = regexp.MustCompile(`(?i)update state \(0x[0-9a-f]+\) ([a-z ]+?), progress: ([0-9.]+) \((\d+) / (\d+)\)`)
)

// read takes one log line; it returns a progress step when the line is one.
func (r *steamResult) read(line string) (DownloadProgress, bool) {
	l := strings.TrimSpace(stampRe.ReplaceAllString(line, ""))
	if l == "" {
		return DownloadProgress{}, false
	}
	low := strings.ToLower(l)
	if r.err == nil && !r.success {
		r.last = l
	}
	loginProgress := r.login.read(l)
	if loginProgress && r.login.err != nil {
		r.err = r.login.err
	}
	if r.login.ready {
		r.loggedIn = true
	}
	if loginProgress {
		return DownloadProgress{State: "login", Line: l}, true
	}
	switch {
	case strings.HasPrefix(low, "success!") && strings.Contains(low, "fully installed"):
		r.success = true
		return DownloadProgress{State: "done", Percent: 100, Line: l}, true
	case strings.HasPrefix(low, "error!") || strings.Contains(low, "failed to install app"):
		r.err = &DownloadError{Kind: errorKind(low), Line: l}
	}
	if m := progressRe.FindStringSubmatch(l); m != nil {
		r.loggedIn = true
		p := DownloadProgress{State: "downloading", Line: l}
		if strings.Contains(strings.ToLower(m[1]), "verif") {
			p.State = "verifying"
		}
		p.Percent, _ = strconv.ParseFloat(m[2], 64)
		p.Done, _ = strconv.ParseInt(m[3], 10, 64)
		p.Total, _ = strconv.ParseInt(m[4], 10, 64)
		return p, true
	}
	return DownloadProgress{}, false
}

func errorKind(low string) string {
	switch {
	case strings.Contains(low, "invalid password") || strings.Contains(low, "invalidpassword"):
		return "password"
	case strings.Contains(low, "two-factor") || strings.Contains(low, "logon denied") || strings.Contains(low, "invalidauthcode") || strings.Contains(low, "invalidloginauthcode") || strings.Contains(low, "steam guard"):
		return "guard"
	case strings.Contains(low, "no subscription") || strings.Contains(low, "not own") || strings.Contains(low, "no license") || strings.Contains(low, "nosubscription"):
		return "ownership"
	case strings.Contains(low, "disk") || strings.Contains(low, "space") || strings.Contains(low, "write failure"):
		return "disk"
	case (strings.Contains(low, "access denied") || strings.Contains(low, "accessdenied") || strings.Contains(low, "permission denied")) &&
		(strings.Contains(low, "writ") || strings.Contains(low, "directory") || strings.Contains(low, "folder") || strings.Contains(low, "file open") || strings.Contains(low, "creat")):
		return "disk"
	case strings.Contains(low, "beta") || strings.Contains(low, "branch"):
		return "branch"
	case strings.Contains(low, "timeout") || strings.Contains(low, "timed out") || strings.Contains(low, "no connection") || strings.Contains(low, "noconnection") || strings.Contains(low, "network") || strings.Contains(low, "connect failed") || strings.Contains(low, "connection failed") || strings.Contains(low, "connection failure") || strings.Contains(low, "connection reset") || strings.Contains(low, "not connected") || strings.Contains(low, "failed to connect") || strings.Contains(low, "could not connect") || strings.Contains(low, "connection lost"):
		return "network"
	case strings.Contains(low, "failed to log in") || strings.Contains(low, "failed to login") || strings.Contains(low, "login failure") || strings.Contains(low, "account disabled"):
		return "login"
	}
	return "other"
}
