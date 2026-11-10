package app

import (
	"errors"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// removeMod is shared by CLI trash, GUI file tasks and subscription removal.
// Link intents are durable before detach. Each completed deletion is published
// immediately; no database transaction encloses filesystem work.
func (a *App) removeMod(ix *Index, m *store.Mod, path string, permanent bool) (*ScanReport, error) {
	links, err := a.IncomingLinks(path)
	if err != nil {
		return nil, err
	}
	err = a.withIncomingLinks(path, "", func() error {
		if permanent {
			if err := a.trashAllowed(path); err != nil {
				return err
			}
			if err := removeModTree(path); err != nil {
				return err
			}
		} else if err := a.recycleTarget(path); err != nil {
			return err
		}
		return a.Store.RecordRemoved(m.ID, m.RealPath, a.Now())
	})
	if partly := (*partlyRemovedError)(nil); errors.As(err, &partly) {
		// The record still says live; let a scan say what is left.
		a.scanLinkedEnvironments(ix, m, links)
	}
	if err != nil {
		return nil, err
	}
	// Reconcile after the lifecycle is complete so scans never see our own
	// transient detached journal as an interrupted operation.
	report, err := a.scanLinkedEnvironments(ix, m, links)
	if err != nil {
		return nil, err
	}
	if report != nil {
		report.GameLinks, err = a.InspectGameLinks()
	}
	return report, err
}

// Rescan every environment whose Mods/ can observe the changed target, including
// shared game roots. This does not select that environment or write its workspace.
func (a *App) scanLinkedEnvironments(ix *Index, m *store.Mod, links []*GameLink) (*ScanReport, error) {
	envs, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	var report *ScanReport
	for _, e := range envs {
		affected := false
		if s := ix.Source(m.SourceID); s != nil && s.EnvID == e.ID {
			affected = true
		}
		for _, l := range links {
			if samePath(l.Game, e.GameRoot) {
				affected = true
			}
		}
		if affected {
			report, err = a.scanEnvironment(e, ScanOptions{GameOnly: true})
			if err != nil {
				return nil, err
			}
		}
	}
	return report, nil
}
