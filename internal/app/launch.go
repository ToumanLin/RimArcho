package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
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

// Launch starts the game (04 §8). It refuses when the workspace has unsaved changes (unless
// force: the saved list is used as is) and always when ModsConfig.xml was written for
// another game version, because the game would discard the list (04 §2.2 rule 5).
func (a *App) Launch(dryRun, force bool) (*LaunchPlan, *LoadoutState, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, nil, err
	}
	e := st.Ix.Env
	if v := st.FileVersionMismatch(); v != "" {
		return nil, st, fmt.Errorf("%w: ModsConfig.xml was saved for %s but the game is %s, so the game would reset the mod list; run archo save first",
			ErrLaunchBlocked, env.MajorMinor(v), st.GameMM())
	}
	if n := len(st.Changes()); n > 0 && !force {
		return nil, st, fmt.Errorf("%w: the workspace has %d unsaved changes; run archo save first, or launch --force to play the saved list",
			ErrLaunchBlocked, n)
	}
	p, err := a.gamePlan(e, e.UserDataRoot)
	if err != nil {
		return nil, st, err
	}
	if dryRun {
		return p, st, nil
	}
	// A pack's links would let the game see its copies next to this list (10 §8.3).
	if _, err := a.removePackLinks(st.Ix, ""); err != nil {
		return nil, st, err
	}
	if err := a.startGame(p); err != nil {
		return nil, st, err
	}
	return p, st, a.Store.AddLaunch(e.ID, a.Now().UTC(), p.Method, p.Command())
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
	extra = append(extra, strings.Fields(a.LaunchArgs(e.ID))...) // 10 §7
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
