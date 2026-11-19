package app

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Launcher starts a process without waiting for it. Tests replace it; nothing in the test
// suite ever starts the game.
type Launcher func(exe string, args []string, dir string) error

// ExecLauncher starts the program detached from RimArcho.
func ExecLauncher(exe string, args []string, dir string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	return cmd.Start()
}

// LaunchPlan is what Launch runs (or would run).
type LaunchPlan struct {
	Method string
	Exe    string
	Args   []string
	Dir    string
}

// Command renders the plan as a command line.
func (p *LaunchPlan) Command() string {
	parts := []string{quoteArg(p.Exe)}
	for _, a := range p.Args {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}

// ErrLaunchBlocked is wrapped by Launch refusals.
var ErrLaunchBlocked = errors.New("launch blocked")

// ErrSteamNotFound is the refusal of a Steam launch with no Steam client to start it; the
// GUI offers to start the game directly instead (16 §2.3).
var ErrSteamNotFound = errors.New("Steam not found")

// Launch plays the current list (09 §2): no game may be running in any environment; then
// the list is written to the game exactly as archo save does (Mods/ links, ModsConfig.xml,
// a version with source "launch" unless the list equals the newest version), the launch is
// recorded and the game started. Nothing blocks on edits since the last write: launching is
// the write. dryRun computes the same preview and writes nothing.
func (a *App) Launch(dryRun bool) (*SaveResult, error) { return a.LaunchWith(dryRun, false) }

// LaunchWith is Launch that, with move, first moves plain Mods/ folders in the way of a
// deployed copy into the lock library (09 §3; --move-duplicates).
func (a *App) LaunchWith(dryRun, move bool) (*SaveResult, error) {
	unlock, err := a.waitLaunch()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := a.checkLaunchArgs(); err != nil {
		return nil, err
	}
	if err := a.checkNoGame(); err != nil {
		return nil, err
	}
	st, err := a.Loadout() // step 2: an external change is attributed first (07 §4.3)
	if err != nil {
		return nil, err
	}
	e := st.Ix.Env
	if err := a.refuseUnhealthyLinks(modsDir(st.Ix)); err != nil {
		return nil, err
	}
	if e.UserDataRoot == "" {
		return nil, errors.New("the Game Config has no user data directory; set one with archo env set --user-data <dir>")
	}
	// The command first: a missing Steam or executable refuses before anything is written.
	p, err := a.gamePlan(e, e.UserDataRoot)
	if err != nil {
		return nil, err
	}
	r, err := a.writeGame(st, dryRun, true, move)
	if err != nil {
		return nil, err
	}
	r.Launch = p
	if dryRun {
		return r, nil
	}
	if err := a.startGame(p); err != nil {
		return r, err
	}
	// Recorded once the game started, so a failed start leaves no launch for logs to match.
	// The record names the plan, the list's fingerprint and the version played (09 §1).
	vn := r.Version
	if r.Unchanged && st.Latest != nil {
		vn = st.Latest.N
	}
	record := &store.LaunchRecord{EnvID: e.ID, GameID: e.GameID, UserDataRoot: e.UserDataRoot, EnvName: e.Name, PlanID: st.Plan.ID, Started: a.Now().UTC(),
		Method: p.Method, Command: p.Command(), Fingerprint: ListFingerprint(st.List), VersionN: vn}
	if err := a.Store.AddLaunchRecord(record); err != nil {
		return r, err
	}
	var enabled []string
	for _, en := range st.List {
		enabled = append(enabled, en.Instance)
	}
	if err := a.Store.CountCreditSession(record.ID, enabled); err != nil {
		log.Printf("game started; cannot count credit session for launch %d: %v", record.ID, err)
	}
	return r, nil
}

func (a *App) startGame(p *LaunchPlan) error {
	// Unity truncates an explicit -logfile instead of preserving the previous run, so the
	// log moves to Player-prev.log first. A launcher that fails started no game: the logs
	// go back where they were (03 §6.3).
	var rotated []*logRotation
	restore := func() {
		for i := len(rotated) - 1; i >= 0; i-- {
			rotated[i].undo()
		}
	}
	for i, arg := range p.Args {
		if arg == "-logfile" && i+1 < len(p.Args) {
			r, err := rotatePlayerLog(p.Args[i+1])
			if err != nil {
				restore()
				return err
			}
			rotated = append(rotated, r)
		}
	}
	launcher := a.Launcher
	if launcher == nil {
		launcher = ExecLauncher
	}
	if err := launcher(p.Exe, p.Args, p.Dir); err != nil {
		restore()
		return err
	}
	for _, r := range rotated {
		r.done()
	}
	return nil
}

// logRotation is one Player.log moved to Player-prev.log before a launch; the older
// Player-prev.log waits beside it until the launch is known.
type logRotation struct{ path, prev, kept string }

// rotatePlayerLog moves path to Player-prev.log beside it (nil when path does not exist).
func rotatePlayerLog(path string) (*logRotation, error) {
	dir := filepath.Dir(path)
	r := &logRotation{path: path, prev: filepath.Join(dir, "Player-prev.log"), kept: filepath.Join(dir, "Player-prev.log.rimarcho-kept")}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	// A kept log left by an interrupted launch is older than Player-prev.log.
	if err := os.Remove(r.kept); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Rename(r.prev, r.kept); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Rename(path, r.prev); err != nil {
		_ = os.Rename(r.kept, r.prev)
		return nil, err
	}
	return r, nil
}

// undo puts both logs back after a failed launch.
func (r *logRotation) undo() {
	if r == nil {
		return
	}
	if err := os.Rename(r.prev, r.path); err == nil {
		_ = os.Rename(r.kept, r.prev)
	}
}

// done forgets the older log once the game started.
func (r *logRotation) done() {
	if r != nil {
		_ = os.Remove(r.kept)
	}
}

// gamePlan is how the environment's game is started with userData as its data folder:
// Steam or the executable, -savedatafolder when it is not the default, the launch arguments.
func (a *App) gamePlan(e *env.Environment, userData string) (*LaunchPlan, error) {
	if err := validateStoredLaunchArgs(e.Name, a.LaunchArgs(e.ID)); err != nil {
		return nil, err
	}
	p := &LaunchPlan{Method: e.Launch, Dir: e.GameRoot}
	goos := a.Detect.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	proton := goos == "linux" && env.IsProtonGame(e.GameRoot)
	defaultUserData := a.DefaultUserData(e.GameRoot)
	if goos == "linux" {
		defaultUserData = a.DefaultUserData()
		if e.Launch == "steam" || proton {
			defaultUserData = a.DefaultUserData(e.GameRoot)
		}
	}
	if proton && e.Launch != "steam" {
		return nil, fmt.Errorf("%w: Proton games must launch through Steam; use archo env set --launch steam", ErrLaunchBlocked)
	}
	var extra []string
	if userData != "" && env.PathKey(userData) != env.PathKey(defaultUserData) {
		if proton {
			return nil, fmt.Errorf("%w: Proton games do not support own or custom saves yet; use archo env set --saves shared", ErrLaunchBlocked)
		}
		extra = append(extra, "-savedatafolder="+userData, "-logfile", filepath.Join(userData, "Player.log"))
	}
	argv, err := storedLaunchArgs(a.LaunchArgs(e.ID))
	if err != nil {
		return nil, err
	}
	extra = append(extra, argv...) // 16 §2.14
	if proton {
		for _, arg := range extra {
			if strings.HasPrefix(strings.ToLower(arg), "-savedatafolder") {
				return nil, fmt.Errorf("%w: Proton games do not support -savedatafolder yet; remove it from the launch arguments", ErrLaunchBlocked)
			}
		}
	}
	switch e.Launch {
	case "steam":
		p.Exe, p.Args = env.SteamCommand(a.Detect)
		if p.Exe == "" {
			if proton {
				return nil, fmt.Errorf("%w: Steam is required for Proton games; install Steam or add it to PATH", ErrLaunchBlocked)
			}
			return nil, fmt.Errorf("%w: %w; use archo env set --launch direct", ErrLaunchBlocked, ErrSteamNotFound)
		}
		p.Args = append(p.Args, "-applaunch", env.WorkshopAppID)
		p.Args = append(p.Args, extra...)
	default:
		p.Method = "direct"
		names := []string{"RimWorldWin64.exe", "RimWorld.exe", "RimWorldLinux", "RimWorldMac.app"}
		if goos == "linux" {
			names = []string{"RimWorldLinux", "RimWorldWin64.exe", "RimWorld.exe", "RimWorldMac.app"}
		}
		for _, name := range names {
			if _, err := os.Stat(filepath.Join(e.GameRoot, name)); err == nil {
				p.Exe = filepath.Join(e.GameRoot, name)
				break
			}
		}
		if p.Exe == "" {
			return nil, fmt.Errorf("%w: no game executable in %s", ErrLaunchBlocked, e.GameRoot)
		}
		p.Args = extra
	}
	return p, nil
}
