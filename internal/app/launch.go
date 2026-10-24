package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// Launch plays the current list (08 §2): no game may be running in any environment; then
// the list is written to the game exactly as archo save does (Mods/ links, ModsConfig.xml,
// a version with source "launch" unless the list equals the newest version), the launch is
// recorded and the game started. Nothing blocks on edits since the last write: launching is
// the write. dryRun computes the same preview and writes nothing.
func (a *App) Launch(dryRun bool) (*SaveResult, error) { return a.LaunchWith(dryRun, false) }

// LaunchWith is Launch that, with move, first moves plain Mods/ folders in the way of a
// deployed copy into the lock library (08 §3; --move-duplicates).
func (a *App) LaunchWith(dryRun, move bool) (*SaveResult, error) {
	if err := a.checkNoGame(); err != nil {
		return nil, err
	}
	st, err := a.Loadout() // step 2: an external change is attributed first (04 §4.3)
	if err != nil {
		return nil, err
	}
	e := st.Ix.Env
	if err := a.refuseUnhealthyLinks(modsDir(st.Ix)); err != nil {
		return nil, err
	}
	if e.UserDataRoot == "" {
		return nil, errors.New("the environment has no user data directory; set one with archo env set --user-data <dir>")
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
	// The record names the plan, the list's fingerprint and the version played (08 §1).
	vn := r.Version
	if r.Unchanged && st.Latest != nil {
		vn = st.Latest.N
	}
	return r, a.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: e.ID, PlanID: st.Plan.ID, Started: a.Now().UTC(),
		Method: p.Method, Command: p.Command(), Fingerprint: ListFingerprint(st.List), VersionN: vn})
}

func (a *App) startGame(p *LaunchPlan) error {
	launcher := a.Launcher
	if launcher == nil {
		launcher = ExecLauncher
	}
	return launcher(p.Exe, p.Args, p.Dir)
}

// gamePlan is how the environment's game is started with userData as its data folder:
// Steam or the executable, -savedatafolder when it is not the default, the launch arguments.
func (a *App) gamePlan(e *env.Environment, userData string) (*LaunchPlan, error) {
	p := &LaunchPlan{Method: e.Launch, Dir: e.GameRoot}
	var extra []string
	if userData != "" && env.PathKey(userData) != env.PathKey(env.DefaultUserDataRoot()) {
		extra = append(extra, "-savedatafolder="+userData)
	}
	extra = append(extra, strings.Fields(a.LaunchArgs(e.ID))...) // 06 §2.14
	switch e.Launch {
	case "steam":
		p.Exe = env.SteamExe(a.Detect)
		if p.Exe == "" {
			return nil, fmt.Errorf("%w: Steam not found; use archo env set --launch direct", ErrLaunchBlocked)
		}
		p.Args = append([]string{"-applaunch", env.WorkshopAppID}, extra...)
	default:
		p.Method = "direct"
		for _, name := range []string{"RimWorldWin64.exe", "RimWorld.exe", "RimWorldLinux", "RimWorldMac.app"} {
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
