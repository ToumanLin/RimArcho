package app

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// UseInstalled binds unresolved packages to the copies offered by MissingMods.
// It preserves their first positions and records the whole batch as one undo step.
// Deployment, when needed, remains part of save/launch.
func (a *App) UseInstalled(packageIDs []string) (*EditResult, error) {
	return a.edit("use installed copies", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		available := map[string]MissingMod{}
		for _, m := range a.MissingMods(st) {
			available[m.PackageID] = m
		}
		targets := map[string]*store.Mod{}
		for _, pkg := range packageIDs {
			p := dependencyPackage(pkg)
			item, ok := available[p]
			if !ok || !item.CanEnable {
				return nil, nil, fmt.Errorf("no usable installed copy for missing package %s", pkg)
			}
			m := st.Ix.ByID[item.Instance]
			if m == nil {
				return nil, nil, fmt.Errorf("installed copy for %s is no longer available", pkg)
			}
			if why := whyNotLoadable(st, m); why != "" {
				return nil, nil, fmt.Errorf("cannot enable %s: %s", item.Ref, why)
			}
			for _, e := range l {
				if other := st.Ix.ByID[e.Instance]; other != nil && dependencyPackage(other.PackageID) == p {
					return nil, nil, fmt.Errorf("cannot enable %s: another copy of pkg:%s is enabled", item.Ref, p)
				}
			}
			targets[p] = m
		}
		out := make(loadout.List, 0, len(l))
		seen := map[string]bool{}
		for _, e := range l {
			if m := targets[dependencyPackage(e.Missing)]; e.Missing != "" && m != nil {
				if seen[m.ID] {
					continue
				}
				seen[m.ID] = true
				e = loadout.Entry{Instance: m.ID}
			}
			out = append(out, e)
		}
		return out, nil, nil
	})
}
