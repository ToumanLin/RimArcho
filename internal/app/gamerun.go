package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// GameRun is a running game of an environment (10 §3.1).
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
	if a.GameProbe != nil {
		return a.GameProbe(e.GameRoot), nil
	}
	return gameProcess(e.GameRoot), nil
}

// RunWhat is what a game run plays: the environment's own list or a pack (10 §8.3).
type RunWhat struct {
	Pack     string // "" for the environment's list
	UserData string // the run's data folder: its ModsConfig.xml, and on 1.7 its Logs/
	Guess    bool   // read from the launch records: the command line was not available
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
	if r.ArgsKnown {
		if r.SaveData == "" || env.PathKey(r.SaveData) == env.PathKey(e.UserDataRoot) {
			return own
		}
		if name := a.packOfDir(r.SaveData); name != "" {
			return RunWhat{Pack: name, UserData: r.SaveData}
		}
		return RunWhat{UserData: r.SaveData}
	}
	at := r.Started
	if at.IsZero() {
		at = a.Now()
	}
	if m, t := a.Store.LaunchBefore(e.ID, at.Add(time.Minute)); strings.HasPrefix(m, "pack:") && at.Sub(t) < launchSlack {
		name := strings.TrimPrefix(m, "pack:")
		return RunWhat{Pack: name, UserData: filepath.Join(a.packPath(name), "userdata"), Guess: true}
	}
	return own
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
		if p := env.PlayerLog(w.UserData, st.GameMM()); p != "" {
			return p
		}
	}
	cur, _ := a.LogPaths(st)
	return cur
}
