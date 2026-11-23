package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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
// or marking it seen (06 §4). A startup game path is stronger evidence than a launch
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
			es, err := a.Store.Envs()
			if err != nil {
				return false, err
			}
			siblings := 0
			for _, e := range es {
				if e.GameID == st.Ix.Env.GameID {
					siblings++
				}
			}
			if siblings < 2 {
				return true, nil
			}
		} else {
			if e := a.envWithGame(root); e != nil {
				return false, fmt.Errorf("log belongs to Game Config %s", e.Name)
			}
			// A game folder no environment has (a log someone sent, a corpus log, or this
			// game before it moved): unknown owner, not another environment's. It can be read
			// and is attributed against this environment, but it is not counted and requests
			// no analysis of another game version.
			return false, nil
		}
	}
	if fi, err := os.Stat(path); err == nil {
		if o := a.waylithLogOwner(st.Ix.Env, path, fi.ModTime()); o != nil {
			return false, fmt.Errorf("log belongs to Game Config %s", o.Name)
		}
		src := a.logSource(st, path, fi.ModTime())
		if src.EnvID != "" && src.EnvID != st.Ix.Env.ID {
			if es, err := a.Store.Envs(); err == nil {
				for _, e := range es {
					if e.ID == src.EnvID {
						return false, fmt.Errorf("log belongs to Game Config %s", e.Name)
					}
				}
			}
			return false, fmt.Errorf("log belongs to another Game Config")
		}
		return src.EnvID == st.Ix.Env.ID, nil
	}
	return false, nil
}

// waylithLogOwner is the 1.7+ Game Config whose launch wrote a Waylith-format log that
// a pre-1.7 Game Config would otherwise read: both installs share the default folder, and
// 1.6 never writes that format (VM round 2026-11-22). The owner is the Game Config of the
// newest Waylith launch before the file was written, as for own-folder 1.7 logs (03 §6.3);
// nil when e is itself 1.7+, the log is not Waylith's, or no Waylith launch precedes it.
func (a *App) waylithLogOwner(e *env.Environment, path string, mod time.Time) *env.Environment {
	if isWaylith(e) || !waylithLogFile(path) {
		return nil
	}
	envs, err := a.Store.Envs()
	if err != nil {
		return nil
	}
	var ids []string
	for _, o := range envs {
		if isWaylith(o) {
			ids = append(ids, o.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	r, err := a.Store.LaunchRecordBefore(ids, mod)
	if err != nil || r == nil || r.EnvID == e.ID {
		return nil
	}
	for _, o := range envs {
		if o.ID == r.EnvID {
			return o
		}
	}
	return nil
}

// waylithLogFile says whether path starts with the 1.7 log header (06 §1: within the
// first 50 lines).
func waylithLogFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(io.LimitReader(f, 64<<10))
	for i := 0; i < 50 && sc.Scan(); i++ {
		if strings.HasPrefix(sc.Text(), "=== Waylith log ===") {
			return true
		}
	}
	return false
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
// against this environment, its only candidate, and the output says so (06 §4).
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
// (06 §2); without it "static constructor of T" names no mod (MCP eval c3, 2026-11-02).
// path "" is pasted text of version textVersion. A log of unknown owner (no record says who
// wrote it, or its game folder is no environment's) is attributed against this environment,
// its only candidate, so it gets this environment's analysis too, but never an analysis of
// another game version. Another environment's log is refused (06 §4; MCP eval 2026-11-04).
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
