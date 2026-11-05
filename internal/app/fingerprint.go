package app

import (
	"crypto/sha256"
	"fmt"
	"slices"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// ListFingerprint is the identity of an enabled list in order (09 §1): what a launch
// record keeps, and what a log's launch is compared with to tell whether the plan changed
// since (06 §4). Missing entries count by their token.
func ListFingerprint(l loadout.List) string { return fingerprintOf(entryKeys(l)) }

// SetFingerprint is the identity of the enabled set, order left out: the slot of an
// analysis run (05 §3). Plans with the same enabled mods share a slot.
func SetFingerprint(l loadout.List) string {
	keys := entryKeys(l)
	slices.Sort(keys)
	return fingerprintOf(slices.Compact(keys))
}

// TokenFingerprint is ListFingerprint for a list known only by its package ids (a pack's).
func TokenFingerprint(tokens []string) string { return fingerprintOf(tokens) }

func entryKeys(l loadout.List) []string {
	keys := make([]string, 0, len(l))
	for _, e := range l {
		keys = append(keys, e.Key())
	}
	return keys
}

func fingerprintOf(keys []string) string {
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s\n", k)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:24]
}

// PlanState is st as the plan p sees it: the same environment, mods and ModsConfig.xml,
// with p's workspace as the list. The current plan returns st itself. Nothing is written.
func (a *App) PlanState(st *LoadoutState, p *store.Plan) (*LoadoutState, error) {
	if p == nil || st.Plan == nil || p.ID == st.Plan.ID {
		return st, nil
	}
	ws, err := a.Store.Workspace(p.ID)
	if err != nil {
		return nil, err
	}
	c := *st
	c.Plan, c.Notices = p, nil
	if c.Latest, err = a.Store.LatestVersion(p.ID); err != nil {
		return nil, err
	}
	if ws == nil { // a plan with versions only starts from its newest (planWorkspace)
		ws = &store.Workspace{PlanID: p.ID, Entries: loadout.List(nil).Encode()}
		if c.Latest != nil {
			ws.Entries, ws.OriginVersion, ws.OriginKind = c.Latest.Entries, c.Latest.N, c.Latest.Source
		}
	}
	c.WS = ws
	c.List = c.readList(ws.Entries)
	return &c, nil
}

// planSetKeys are the enabled-set fingerprints of every plan of st's environment: the
// analysis slots worth keeping.
func (a *App) planSetKeys(st *LoadoutState) []string {
	keys := []string{SetFingerprint(st.List)}
	ps, err := a.Store.Plans()
	if err != nil {
		return keys
	}
	for _, p := range ps {
		if st.Plan != nil && p.ID == st.Plan.ID {
			continue
		}
		if ps, err := a.PlanState(st, p); err == nil {
			keys = append(keys, SetFingerprint(ps.List))
		}
	}
	return keys
}
