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
		if es, err := a.Store.Envs(); err == nil {
			for _, e := range es {
				if env.PathKey(root) == env.PathKey(e.GameRoot) {
					return false, fmt.Errorf("log belongs to environment %s", e.Name)
				}
			}
		}
		// A game folder no environment has (a log someone sent, a corpus log, or this
		// game before it moved): unknown owner, not another environment's. It can be read
		// but is not counted and requests no analysis.
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
