package app

import "fmt"

// All RimArcho processes sharing this data directory serialize launch and credit
// handoff. OS locks release on process death; this is distinct from per-Mods link locks.
// Steam/external launches cannot honor it, so handoff also probes before mutations.
func (a *App) lockLaunch() (func(), error) {
	unlock, err := lockGameLinks(realOrAbs(a.Home))
	if err != nil {
		return nil, fmt.Errorf("another launch or credit replacement is running: %w", err)
	}
	return unlock, nil
}
