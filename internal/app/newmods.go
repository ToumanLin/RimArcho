package app

import (
	"slices"
	"time"
)

// NewFor is how long a Mod stays new after a scan first finds it (16 §2.5, §2.9).
const NewFor = 7 * 24 * time.Hour

// Added is when each Mod (lineage) was first found: its earliest copy's first scan. A
// locked version or another copy of a Mod already there does not make the Mod newer.
// Archo Monitor and the generated Mods (IsGenerated) are left out (zero): RimArcho installs
// them, the player did not add them.
func Added(st *LoadoutState) map[string]time.Time {
	out := make(map[string]time.Time, len(st.Ix.ByLineage))
	for id, mods := range st.Ix.ByLineage {
		if slices.ContainsFunc(mods, IsMonitor) || slices.ContainsFunc(mods, IsGenerated) {
			continue
		}
		var first time.Time
		for _, m := range mods {
			if !m.FirstSeen.IsZero() && (first.IsZero() || m.FirstSeen.Before(first)) {
				first = m.FirstSeen
			}
		}
		out[id] = first
	}
	return out
}

// NewSince is the time after which a Mod first found is new: NewFor ago, and never before
// the environment's first finished scan, which finds everything already installed. Without
// a finished scan nothing is new (zero time).
func (a *App) NewSince(gameID string) time.Time {
	sc, err := a.Store.FirstFinishedScan(gameID)
	if err != nil || sc == nil {
		return time.Time{}
	}
	since := a.Now().Add(-NewFor)
	if sc.Finished.After(since) {
		since = sc.Finished
	}
	return since
}

// AddedValues are the added: filter values of a Mod first found at added: new when it is
// (IsNew), then its LibAges.
func AddedValues(added time.Time, isNew bool, now time.Time) []string {
	if isNew {
		return append([]string{"new"}, libAges(added, now)...)
	}
	return libAges(added, now)
}

// IsNew: a Mod first found at added is new (NewSince).
func IsNew(added, since time.Time) bool {
	return !since.IsZero() && added.After(since)
}
