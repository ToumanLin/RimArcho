package app

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// gameVersionName is a game version given where a Game Config name is expected: 1.7, 1.6.4871.
var gameVersionName = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)

// envNamed is the Game Config named name or, when none is and name is a game version, the
// only one whose game has that version (`--env 1.7`, MCP env="1.7"; 13 §2). The error names
// every Game Config with its game version.
func (a *App) envNamed(name string) (*env.Environment, error) {
	e, err := a.Store.EnvByName(name)
	if err != nil || e != nil {
		return e, err
	}
	all, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	var match []*env.Environment
	if gameVersionName.MatchString(name) {
		for _, c := range all {
			v := LiveGameVersion(c)
			if v == name || strings.HasPrefix(v, name+".") || strings.HasPrefix(v, name+" ") {
				match = append(match, c)
			}
		}
	}
	if len(match) == 1 {
		return match[0], nil
	}
	names := make([]string, 0, len(all))
	for _, c := range all {
		names = append(names, fmt.Sprintf("%s (RimWorld %s)", c.Name, env.MajorMinor(LiveGameVersion(c))))
	}
	if len(match) > 1 {
		var ns []string
		for _, c := range match {
			ns = append(ns, c.Name)
		}
		return nil, fmt.Errorf("several Game Configs have RimWorld %s: %s; give one of their names", name, strings.Join(ns, ", "))
	}
	if len(names) == 0 {
		return nil, ErrNoEnv
	}
	return nil, fmt.Errorf("no Game Config named %q; the Game Configs are %s", name, strings.Join(names, ", "))
}

// ResolveEnvName is the name of the Game Config name selects (a name, or a game version
// only one has); name itself when it selects none, so the call's error says why.
func (a *App) ResolveEnvName(name string) string {
	if e, err := a.envNamed(name); err == nil && e != nil {
		return e.Name
	}
	return name
}

// LiveGameVersion is the version in the game folder's Version.txt, else the one stored at
// the last scan: a game Steam updated since shows its new version before the next scan.
func LiveGameVersion(e *env.Environment) string {
	if v := env.ReadGameVersion(e.GameRoot); v != "" {
		return v
	}
	return e.GameVersion
}
