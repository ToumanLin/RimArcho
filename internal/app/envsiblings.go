package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// canonicalRoot resolves existing ancestors too, so a new child of an alias conflicts.
func canonicalRoot(path string) string {
	if path == "" {
		return ""
	}
	if real, err := source.RealPath(path); err == nil {
		return env.PathKey(real)
	}
	abs, _ := filepath.Abs(path)
	parent := filepath.Dir(abs)
	if parent == abs {
		return env.PathKey(abs)
	}
	return env.PathKey(filepath.Join(canonicalRoot(parent), filepath.Base(abs)))
}

func (a *App) checkEnvironmentRoot(self, game, root string) error {
	es, err := a.Store.Envs()
	if err != nil {
		return err
	}
	g, r := canonicalRoot(game), canonicalRoot(root)
	if r != "" && (underDir(r, g) || underDir(g, r)) {
		return fmt.Errorf("user data folder %s overlaps game folder %s; choose a separate --user-data <dir>", root, game)
	}
	for _, e := range es {
		if e.ID == self {
			continue
		}
		o := canonicalRoot(e.UserDataRoot)
		if g == canonicalRoot(e.GameRoot) && r == o {
			return fmt.Errorf("environment %s already uses this installation and user data folder; use --saves own or --user-data <dir>", e.Name)
		}
		if r != "" && o != "" && r != o && (underDir(r, o) || underDir(o, r)) {
			return fmt.Errorf("user data folder %s overlaps environment %s's folder %s; choose a separate --user-data <dir>", root, e.Name, e.UserDataRoot)
		}
	}
	return nil
}

func (a *App) siblingPlanName(name string) (string, error) {
	base := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '_'
		}
		return r
	}, name)
	base = strings.TrimSpace(strings.TrimLeft(base, "-"))
	if base == "" {
		base = "environment"
	}
	if runes := []rune(base); len(runes) > 56 {
		base = string(runes[:56])
	}
	ps, err := a.Store.Plans()
	if err != nil {
		return "", err
	}
	name = store.UniquePlanName(base, func(n string) bool {
		for _, p := range ps {
			if strings.EqualFold(n, p.Name) {
				return true
			}
		}
		return false
	})
	return a.checkPlanName(name, 0)
}

func (a *App) initializeSibling(e, like *env.Environment, clone *LoadoutState) error {
	name, err := a.siblingPlanName(e.Name)
	if err != nil {
		return err
	}
	var list loadout.List
	if clone != nil {
		list = clone.List.Clone()
	} else {
		ix, err := a.loadIndex(e)
		if err != nil {
			return err
		}
		list = emptyEnvironmentList(ix)
	}
	p, err := a.createPlan(e.ID, name, list, &store.Version{Source: "import", Note: "new environment", GameVersion: e.GameVersion})
	if err != nil {
		return err
	}
	if err := a.Store.SetCurrentPlan(e.ID, p.ID); err != nil {
		return err
	}
	if like != nil {
		args, _ := withoutSaveData(a.LaunchArgs(like.ID))
		return a.SetLaunchArgs(e.ID, args)
	}
	return nil
}

// seedSettings shares the pack step-4 rules, while reporting every copy failure.
func seedSettings(from, to string, config bool) ([]string, error) {
	if from == "" {
		return nil, nil
	}
	var copied []string
	if config {
		ents, err := os.ReadDir(filepath.Join(from, "Config"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for _, f := range ents {
			if !f.Type().IsRegular() || strings.EqualFold(f.Name(), "ModsConfig.xml") || !strings.EqualFold(filepath.Ext(f.Name()), ".xml") {
				continue
			}
			cfg := filepath.Join(to, "Config")
			if err := os.MkdirAll(cfg, 0o755); err != nil {
				return copied, err
			}
			fi, err := f.Info()
			if err != nil {
				return copied, err
			}
			err = copyFile(filepath.Join(from, "Config", f.Name()), filepath.Join(cfg, f.Name()), fi)
			if errors.Is(err, os.ErrExist) {
				continue
			}
			if err != nil {
				return copied, err
			}
			copied = append(copied, f.Name())
		}
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return copied, err
	}
	hl, err := copyHugsLib(from, to)
	return append(copied, hl...), err
}

// Retain original quoting in arguments unrelated to the root override.
func withoutSaveData(args string) (string, bool) {
	var parts []string
	start := -1
	var quote rune
	for i, r := range args {
		if start < 0 && !unicode.IsSpace(r) {
			start = i
		}
		if r == '"' || r == '\'' {
			if quote == 0 {
				quote = r
			} else if quote == r {
				quote = 0
			}
		}
		if unicode.IsSpace(r) && quote == 0 && start >= 0 {
			parts = append(parts, args[start:i])
			start = -1
		}
	}
	if start >= 0 {
		parts = append(parts, args[start:])
	}
	out := []string{}
	omitted := false
	for i := 0; i < len(parts); i++ {
		key := strings.ToLower(strings.Trim(parts[i], "\"'"))
		if key == "-savedatafolder" || strings.HasPrefix(key, "-savedatafolder=") {
			omitted = true
			if key == "-savedatafolder" && i+1 < len(parts) {
				i++
			}
			continue
		}
		out = append(out, parts[i])
	}
	return strings.Join(out, " "), omitted
}

// CloneArgsOmitted reports whether --like discards a stored data-root override.
func (a *App) CloneArgsOmitted(name string) bool {
	e, err := a.Store.EnvByName(strings.TrimPrefix(name, "env:"))
	if err != nil || e == nil {
		return false
	}
	_, omitted := withoutSaveData(a.LaunchArgs(e.ID))
	return omitted
}

func emptyEnvironmentList(ix *Index) loadout.List {
	var list loadout.List
	for _, m := range installedOfficial(ix.Mods) {
		list = append(list, loadout.Entry{Instance: m.ID})
	}
	sort.SliceStable(list, func(i, j int) bool {
		return order.CompareOfficial(ix.ByID[list[i].Instance].PackageID, ix.ByID[list[j].Instance].PackageID) < 0
	})
	return list
}
