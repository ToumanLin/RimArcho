package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// Unity prints this before loading mods. Do not infer a game root from paths in error
// messages or stack frames: those may refer to a mod author's build machine.
var monoGamePath = regexp.MustCompile(`(?m)^Mono path\[0\] = '([^'\r\n]+)'`)

// logGameRoot reads only the startup header. Relative placeholders (the synthetic
// fixture's 'x', for example) cannot identify a game. The 1.7 Waylith fixtures have a
// version line but no game path; those use the launch records instead.
func logGameRoot(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, 64<<10))
	m := monoGamePath.FindSubmatch(b)
	if m == nil {
		return ""
	}
	p := filepath.FromSlash(strings.ReplaceAll(string(m[1]), `\`, "/"))
	if !filepath.IsAbs(p) || !strings.EqualFold(filepath.Base(p), "Managed") ||
		!strings.HasSuffix(strings.ToLower(filepath.Base(filepath.Dir(p))), "_data") {
		return ""
	}
	return filepath.Dir(filepath.Dir(p))
}

// CheckLogOwner excludes another game's log before analysis, attribution, cache reads
// or marking it seen (03 §6.4). A startup game path is stronger evidence than a launch
// matched by time (the player may have launched from Steam since). Version alone is
// insufficient: this game directory may have been upgraded since writing the log.
// Unknown ownership retains the existing source-unknown/stale handling.
func (a *App) CheckLogOwner(st *LoadoutState, path string) error {
	_, err := a.logOwner(st, path)
	return err
}

// logOwner distinguishes positive ownership from an unknown owner. Both may be read,
// but only a positive match may request analysis for another game version.
func (a *App) logOwner(st *LoadoutState, path string) (own bool, err error) {
	if path == "" {
		return false, nil
	}
	if root := logGameRoot(path); root != "" {
		if env.PathKey(root) == env.PathKey(st.Ix.Env.GameRoot) {
			return true, nil
		}
		if e := a.envWithGame(root); e != nil {
			return false, fmt.Errorf("log belongs to environment %s", e.Name)
		}
		// A game folder no environment has (a log someone sent, a corpus log, or this
		// game before it moved): unknown owner, not another environment's. It can be read
		// and is attributed against this environment, but it is not counted and requests
		// no analysis of another game version.
		return false, nil
	}
	if fi, err := os.Stat(path); err == nil {
		src := a.logSource(st, path, fi.ModTime())
		if src.EnvID != "" && src.EnvID != st.Ix.Env.ID {
			if es, err := a.Store.Envs(); err == nil {
				for _, e := range es {
					if e.ID == src.EnvID {
						return false, fmt.Errorf("log belongs to environment %s", e.Name)
					}
				}
			}
			return false, fmt.Errorf("log belongs to another environment")
		}
		return src.EnvID == st.Ix.Env.ID, nil
	}
	return false, nil
}

// envWithGame is the registered environment whose game folder is root (nil: none).
func (a *App) envWithGame(root string) *env.Environment {
	es, err := a.Store.Envs()
	if err != nil {
		return nil
	}
	for _, e := range es {
		if env.PathKey(root) == env.PathKey(e.GameRoot) {
			return e
		}
	}
	return nil
}

// UnregisteredLogGame is the game folder a log's startup header names when it is neither
// this environment's nor any other registered environment's ("" otherwise): a log someone
// sent, a corpus log, or this game before its folder moved. Such a log is attributed
// against this environment, its only candidate, and the output says so (03 §6.4).
func (a *App) UnregisteredLogGame(st *LoadoutState, path string) string {
	if path == "" {
		return ""
	}
	root := logGameRoot(path)
	if root == "" || env.PathKey(root) == env.PathKey(st.Ix.Env.GameRoot) || a.envWithGame(root) != nil {
		return ""
	}
	return root
}

// EnsureLogFileAnalysis checks ownership before considering the log's version.
func (a *App) EnsureLogFileAnalysis(st *LoadoutState, path string, progress func(string)) error {
	own, err := a.logOwner(st, path)
	if err != nil {
		return err
	}
	if !own {
		return nil
	}
	return a.EnsureLogAnalysis(st, LogVersionOf(path), progress)
}

// EnsureAttributionAnalysis is what `archo log` (and MCP analyze_log) runs before
// attributing a log: EnsureLogFileAnalysis, and for logs of this environment's version the
// environment's analysis too. Assembly types and Harmony IDs come only from the analysis
// (03 §6.2); without it "static constructor of T" names no mod (MCP eval c3, 2026-11-02).
// path "" is pasted text of version textVersion. A log of unknown owner (no record says who
// wrote it, or its game folder is no environment's) is attributed against this environment,
// its only candidate, so it gets this environment's analysis too, but never an analysis of
// another game version. Another environment's log is refused (03 §6.4; MCP eval 2026-11-04).
func (a *App) EnsureAttributionAnalysis(st *LoadoutState, path, textVersion string, progress func(string)) error {
	version := textVersion
	if path != "" {
		own, err := a.logOwner(st, path)
		if err != nil {
			return err
		}
		version = LogVersionOf(path)
		if !own {
			if mm := LogMM(version); a.GameVersion == "" && mm != "" && mm != st.GameMM() {
				return nil // attributed with whatever facts of its version are stored
			}
			_, err := a.EnsureAnalysis(progress)
			return err
		}
	}
	if mm := LogMM(version); a.GameVersion == "" && mm != "" && mm != st.GameMM() {
		return a.EnsureLogAnalysis(st, version, progress)
	}
	_, err := a.EnsureAnalysis(progress)
	return err
}
