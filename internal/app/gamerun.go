package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// GameRun is a running game of an environment (09 §4).
type GameRun struct {
	PID     int
	Exe     string
	Started time.Time
	// SaveData is the -savedatafolder the game was started with ("" for the default);
	// ArgsKnown says whether the command line could be read at all.
	SaveData  string
	ArgsKnown bool
}

// SaveDataArg finds -savedatafolder=<path> in a command line (the path may be quoted, or
// the whole argument may be).
func SaveDataArg(cmdline string) string {
	low := strings.ToLower(cmdline)
	i := strings.Index(low, "-savedatafolder=")
	if i < 0 {
		return ""
	}
	quoted := i > 0 && cmdline[i-1] == '"'
	rest := cmdline[i+len("-savedatafolder="):]
	if strings.HasPrefix(rest, `"`) {
		rest, quoted = rest[1:], true
	}
	end := len(rest)
	if quoted {
		if j := strings.IndexByte(rest, '"'); j >= 0 {
			end = j
		}
	} else if j := strings.IndexAny(rest, " \t"); j >= 0 {
		end = j
	}
	return strings.TrimSpace(rest[:end])
}

// GameRunning reports the environment's running game, if any.
func (a *App) GameRunning() (*GameRun, error) {
	e, err := a.Env()
	if err != nil {
		return nil, err
	}
	return a.probe()(e.GameRoot), nil
}

// probe returns how to find the running game of a game folder: GameProbe in tests, else a
// match against the process table, listed once on first use and shared by the calls.
func (a *App) probe() func(gameRoot string) *GameRun {
	if a.GameProbe != nil {
		return a.GameProbe
	}
	var procs []*GameRun
	listed := false
	return func(gameRoot string) *GameRun {
		if !listed {
			procs, listed = gameProcesses(), true
		}
		return processIn(procs, gameRoot)
	}
}

// processIn picks the process whose executable lies in gameRoot.
func processIn(procs []*GameRun, gameRoot string) *GameRun {
	if gameRoot == "" {
		return nil
	}
	root := env.PathKey(gameRoot)
	for _, r := range procs {
		if underDir(env.PathKey(filepath.Dir(r.Exe)), root) {
			return r
		}
	}
	return nil
}

// underDir reports whether the cleaned path p is dir or inside it ("D:/RimWorld2" is not in
// "D:/RimWorld").
func underDir(p, dir string) bool {
	if p == dir {
		return true
	}
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return strings.HasPrefix(p, dir)
}

// RunningGame is a game running in any registered environment (09 §1, §4).
type RunningGame struct {
	Env  *env.Environment
	Run  *GameRun
	What RunWhat
}

// Describe names what runs for a refusal or a status line: the environment and the list or
// pack it plays.
func (g *RunningGame) Describe(loc *time.Location) string {
	what := g.What.PlanText()
	if g.What.Pack != "" {
		what = "pack " + g.What.Pack
		if g.What.Guess {
			what += " (from the launch records)"
		}
	} else if g.What.UserData != "" && env.PathKey(g.What.UserData) != env.PathKey(g.Env.UserDataRoot) {
		what = "data folder " + g.What.UserData
	}
	s := fmt.Sprintf("environment %s is running %s (pid %d", g.Env.Name, what, g.Run.PID)
	if !g.Run.Started.IsZero() {
		if loc == nil {
			loc = time.Local
		}
		s += ", since " + g.Run.Started.In(loc).Format("15:04")
	}
	return s + ")"
}

// GameRunningError refuses a launch while a game runs: only one game runs at a time (09 §1).
type GameRunningError struct {
	Game *RunningGame
	Loc  *time.Location
}

func (e *GameRunningError) Error() string {
	return fmt.Sprintf("%v: a game is already running: %s; only one game runs at a time, close it first", ErrLaunchBlocked, e.Game.Describe(e.Loc))
}

func (e *GameRunningError) Unwrap() error { return ErrLaunchBlocked }

// RunningGame finds a running game in any registered environment, the current one first: a
// process whose executable is under an environment's game folder (09 §4). nil when none.
func (a *App) RunningGame() (*RunningGame, error) {
	envs, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	if cur, err := a.Env(); err == nil {
		for i, e := range envs {
			if e.ID == cur.ID {
				envs[0], envs[i] = envs[i], envs[0]
				break
			}
		}
	}
	probe := a.probe()
	for _, e := range envs {
		if r := probe(e.GameRoot); r != nil {
			return &RunningGame{Env: e, Run: r, What: a.WhatRuns(e, r)}, nil
		}
	}
	return nil, nil
}

// checkNoGame refuses while any environment's game runs (09 §1).
func (a *App) checkNoGame() error {
	g, err := a.RunningGame()
	if err != nil {
		return err
	}
	if g != nil {
		return &GameRunningError{Game: g, Loc: a.Loc}
	}
	return nil
}

// RunWhat is what a game run plays: one of the environment's plans or a pack (10 §3).
type RunWhat struct {
	Pack     string // "" for the environment's list
	UserData string // the run's data folder: its ModsConfig.xml, and on 1.7 its Logs/
	Guess    bool   // read from the launch records: the command line was not available
	// Plan names the plan played, from the launch records (09 §4) or, for a game started
	// elsewhere, the plan last written to the game; "" when neither says.
	Plan string
	// PlanWritten: Plan comes from the write record, not from a launch.
	PlanWritten bool
}

// PlanText names the plan a run plays: "plan:<name>", "plan:<name> (last written to the
// game)", or "its list" when the records do not say.
func (w RunWhat) PlanText() string {
	switch {
	case w.Plan == "":
		return "its list"
	case w.PlanWritten:
		return "plan:" + w.Plan + " (last saved)"
	}
	return "plan:" + w.Plan
}

// withPlan fills the plan of an own-list run started at at from the records.
func (a *App) withPlan(e *env.Environment, w RunWhat, at time.Time) RunWhat {
	src := a.sourceAt(e, nil, at.Add(time.Minute), true)
	if !src.Unknown && src.Plan != "" && (src.EnvID == "" || src.EnvID == e.ID) {
		w.Plan, w.PlanWritten = src.Plan, src.ByWrite
	}
	return w
}

// launchSlack is how long before the process start a launch record may be: Steam takes a
// while to start the game.
const launchSlack = 10 * time.Minute

// WhatRuns says what the running game r plays.
func (a *App) WhatRuns(e *env.Environment, r *GameRun) RunWhat {
	own := RunWhat{UserData: e.UserDataRoot}
	if r == nil {
		return own
	}
	at := r.Started
	if at.IsZero() {
		at = a.Now()
	}
	if r.ArgsKnown {
		if r.SaveData == "" || env.PathKey(r.SaveData) == env.PathKey(e.UserDataRoot) {
			return a.withPlan(e, own, at)
		}
		if name := a.packOfDir(r.SaveData); name != "" {
			return RunWhat{Pack: name, UserData: r.SaveData}
		}
		return RunWhat{UserData: r.SaveData}
	}
	if m, t := a.Store.LaunchBefore(e.ID, at.Add(time.Minute)); strings.HasPrefix(m, "pack:") && at.Sub(t) < launchSlack {
		name := strings.TrimPrefix(m, "pack:")
		return RunWhat{Pack: name, UserData: filepath.Join(a.packPath(name), "userdata"), Guess: true}
	}
	return a.withPlan(e, own, at)
}

// RunOfLog says which launch wrote a log last modified at t: a pack's name, or "" for the
// environment's list (also when nothing was launched from RimArcho).
func (a *App) RunOfLog(envID string, t time.Time) string {
	m, _ := a.Store.LaunchBefore(envID, t)
	if name, ok := strings.CutPrefix(m, "pack:"); ok {
		return name
	}
	return ""
}

// packOfDir returns the pack whose userdata folder dir is, or "".
func (a *App) packOfDir(dir string) string {
	root := env.PathKey(a.PacksDir())
	d := env.PathKey(dir)
	rel, ok := strings.CutPrefix(d, root)
	if !ok || rel == "" {
		return ""
	}
	parts := strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) != 2 || parts[1] != "userdata" {
		return ""
	}
	// the key is lower case: the pack's real name comes from the folder
	if ents, err := os.ReadDir(a.PacksDir()); err == nil {
		for _, de := range ents {
			if strings.EqualFold(de.Name(), parts[0]) {
				return de.Name()
			}
		}
	}
	return parts[0]
}

// RunLog is the Player.log a run writes: on 1.7 under its data folder's Logs/, before 1.7
// the one Unity keeps (the environment's).
func (a *App) RunLog(st *LoadoutState, w RunWhat) string {
	if w.UserData != "" && env.PathKey(w.UserData) != env.PathKey(st.Ix.Env.UserDataRoot) {
		if p := env.PlayerLog(w.UserData, st.GameMM()); p != "" && a.CheckLogOwner(st, p) == nil {
			return p
		}
	}
	cur, _ := a.LogPaths(st)
	return cur
}
