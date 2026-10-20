package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Changes (06 §4.1): the changes of the last scans in words, newest first; each update can
// show the semantic files it changed.

// ChangeScans is the number of scans shown.
const ChangeScans = 10

// ChangeEvent is one change of one mod.
type ChangeEvent struct {
	Event *store.Event `json:"-"`
	ID    int64        `json:"id"`
	ModID string       `json:"modId"`
	Mod   string       `json:"mod"`   // its name
	Text  string       `json:"text"`  // the change in words
	Files bool         `json:"files"` // FileDiff has something to show
}

// ChangeScan is one scan with its changes, most interesting first.
type ChangeScan struct {
	Time    string        `json:"time"`
	Summary string        `json:"summary"`
	Events  []ChangeEvent `json:"events"`
}

// Changes lists the last scans of the state's environment (none: no scan yet).
func Changes(a *app.App, st *app.LoadoutState) ([]ChangeScan, error) {
	ix := st.Ix
	scans, err := a.Store.Scans(ix.Env.ID, ChangeScans)
	if err != nil || len(scans) == 0 {
		return nil, err
	}
	evs, err := a.Store.Events(store.EventFilter{EnvID: ix.Env.ID, MinScan: scans[len(scans)-1].ID})
	if err != nil {
		return nil, err
	}
	by := map[int64][]*store.Event{}
	for _, e := range evs {
		by[e.ScanID] = append(by[e.ScanID], e)
	}
	name := func(id string) string {
		if x := ix.ByID[id]; x != nil {
			return x.DisplayName()
		}
		return T("a removed mod")
	}
	var out []ChangeScan
	for _, s := range scans {
		es := by[s.ID]
		counts := map[string]int{}
		for _, e := range es {
			counts[e.Kind]++
		}
		cs := ChangeScan{Time: a.FormatTime(s.Started), Summary: ChangeSummary(counts)}
		if cs.Summary == "" {
			cs.Summary = T("nothing changed")
		}
		sort.SliceStable(es, func(i, j int) bool {
			if es[i].Kind != es[j].Kind {
				return KindOrder(es[i].Kind) < KindOrder(es[j].Kind)
			}
			return strings.ToLower(name(es[i].InstanceID)) < strings.ToLower(name(es[j].InstanceID))
		})
		for _, e := range es {
			cs.Events = append(cs.Events, ChangeEvent{Event: e, ID: e.ID, ModID: e.InstanceID, Mod: name(e.InstanceID), Text: EventText(e.Kind),
				Files: e.Before != 0 && e.After != 0 && e.Before != e.After})
		}
		out = append(out, cs)
	}
	return out, nil
}

// KindOrder puts what a player most wants to know first.
func KindOrder(k string) int {
	for i, x := range []string{identity.EvUpdated, identity.EvPackageIDChanged, identity.EvRenamed, identity.EvDiscovered,
		identity.EvDisappeared, identity.EvReappeared, identity.EvDependencies, identity.EvLoadRules, identity.EvVersionSupport,
		identity.EvAuthorChanged, identity.EvMoved, identity.EvCopied} {
		if k == x {
			return i
		}
	}
	return 99
}

// ChangeSummary: "12 mods updated · 1 changed its package id" (06 §4.1).
func ChangeSummary(counts map[string]int) string {
	var kinds []string
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return KindOrder(kinds[i]) < KindOrder(kinds[j]) })
	var parts []string
	for _, k := range kinds {
		n := counts[k]
		switch k {
		case identity.EvUpdated:
			parts = append(parts, Tf("%d mods updated", n))
		case identity.EvDiscovered:
			parts = append(parts, Tf("%d new mods", n))
		case identity.EvDisappeared:
			parts = append(parts, Tf("%d mods gone", n))
		case identity.EvPackageIDChanged:
			parts = append(parts, Tf("%d changed their package id", n))
		default:
			parts = append(parts, fmt.Sprintf("%d %s", n, EventText(k)))
		}
	}
	return strings.Join(parts, " · ")
}

// FileDiff lists the semantic files an event changed: "+ new", "~ changed", "- removed".
func FileDiff(a *app.App, e *store.Event) []string {
	if e.Before == 0 || e.After == 0 || e.Before == e.After {
		return []string{T("(no file-level difference recorded for this event)")}
	}
	x, err1 := a.Store.SemanticSet(e.Before)
	y, err2 := a.Store.SemanticSet(e.After)
	if err1 != nil || err2 != nil {
		return []string{T("(snapshot no longer stored)")}
	}
	var out []string
	for p, h := range y {
		if old, ok := x[p]; !ok {
			out = append(out, "+ "+p)
		} else if old != h {
			out = append(out, "~ "+p)
		}
	}
	for p := range x {
		if _, ok := y[p]; !ok {
			out = append(out, "- "+p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][2:] < out[j][2:] })
	if len(out) == 0 {
		return []string{T("(semantic files unchanged: only other files changed)")}
	}
	if len(out) > 20 {
		out = append(out[:20], Tf("… %d more", len(out)-20))
	}
	return out
}
