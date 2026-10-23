package app

import (
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Older game-only scans registered managed links as separate local instances. Fold
// those records back into the shared instance of the exact same directory. Package
// IDs and similar content alone are deliberately insufficient to merge two mods.
func (a *App) repairSharedLinkInstances(mods []*store.Mod, sources []*source.Source) (bool, error) {
	shared := map[string]*store.Mod{}
	bySource := map[string]*source.Source{}
	for _, s := range sources {
		bySource[s.ID] = s
	}
	for _, m := range mods {
		s := bySource[m.SourceID]
		if s != nil && s.EnvID == store.Shared && s.Status == source.StatusOK && m.Status == identity.Live && m.RealPath != "" {
			shared[env.PathKey(m.RealPath)] = m
		}
	}
	replace := map[string]string{}
	for _, m := range mods {
		if m.Kind != source.Local || !managedLink(m.Rel) || m.RealPath == "" {
			continue
		}
		canonical := shared[env.PathKey(m.RealPath)]
		if canonical == nil {
			continue
		}
		s := bySource[canonical.SourceID]
		if sameDir(filepath.Join(s.Root, canonical.Rel), m.RealPath) {
			replace[m.ID] = canonical.ID
		}
	}
	if len(replace) == 0 {
		return false, nil
	}
	return true, a.Store.MergeLinkedInstances(replace)
}
