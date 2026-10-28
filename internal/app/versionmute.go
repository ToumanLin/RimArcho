package app

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// SetVersionMute changes a global preference for packages resolved from mod references.
// Resolution and validation finish before the batch is written.
func (a *App) SetVersionMute(refs []string, mm string, muted bool) error {
	if len(refs) == 0 {
		return fmt.Errorf("give at least one mod reference")
	}
	st, err := a.Loadout()
	if err != nil {
		return err
	}
	if mm == "" {
		mm = st.GameMM()
	}
	if env.MajorMinor(mm) != mm || mm == "" {
		return fmt.Errorf("%q is not a game minor version: write 1.7", mm)
	}
	var packages []string
	for _, ref := range refs {
		// Explicit package references also allow undo after the mod was removed.
		pkg, explicit := strings.CutPrefix(ref, "pkg:")
		if !explicit {
			m, err := st.Ix.ResolveOne(ref)
			if err != nil {
				return err
			}
			pkg = m.PackageID
		}
		pkg = store.VersionMuteKey(pkg)
		if pkg == "" {
			return fmt.Errorf("%q has no package id", ref)
		}
		packages = append(packages, pkg)
	}
	return a.Store.SetVersionMutes(packages, mm, muted)
}
