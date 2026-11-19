package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
)

// AddOffer is what a finished download can add to the current list (16 §2.9): the Mods it
// brought that the list does not hold yet. Empty Refs: nothing to offer (yet: a Steam
// download appears after the next scan).
type AddOffer struct {
	Refs  []string `json:"refs"`
	Names []string `json:"names"`
	List  string   `json:"list"`
}

// DownloadedOffer finds the live copies of a finished subscription task: its Workshop items
// (ws:<id>) that succeeded and its target reference. Copies whose package the current list
// already holds, in any copy, are left out.
func DownloadedOffer(a *app.App, t app.Task) (*AddOffer, error) {
	o := &AddOffer{}
	st, err := a.Loadout()
	if err != nil {
		return o, err
	}
	if p, err := a.CurrentPlan(); err == nil && p != nil {
		o.List = p.Name
	}
	ws := map[string]bool{}
	for _, it := range t.Items {
		if id, ok := strings.CutPrefix(it.Ref, "ws:"); ok && it.State == app.ItemOK {
			ws[id] = true
		}
	}
	have := map[string]bool{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			have[strings.ToLower(m.PackageID)] = true
		}
	}
	for _, m := range st.Ix.Mods {
		if m.Status != identity.Live || have[strings.ToLower(m.PackageID)] {
			continue
		}
		ref := st.Ix.Ref(m.ID)
		if !(ws[m.WorkshopID()] || (t.Target != "" && t.Target == ref)) {
			continue
		}
		have[strings.ToLower(m.PackageID)] = true
		o.Refs = append(o.Refs, ref)
		o.Names = append(o.Names, m.DisplayName())
	}
	return o, nil
}

// AddDownloaded enables what DownloadedOffer offered, at the end of the current list, as one
// undoable edit; the next sort or Can it run places it.
func AddDownloaded(a *app.App, refs []string) (*Result, error) {
	r, err := a.Enable(refs, app.Placement{})
	if err != nil {
		return nil, err
	}
	st, _ := a.Loadout()
	what := Plural(len(refs), "mod")
	if len(refs) == 1 && st != nil {
		if m, err := st.Ix.ResolveOne(refs[0]); err == nil && m != nil {
			what = m.DisplayName()
		}
	}
	return &Result{Text: Tf("enabled %s", what), Undo: len(r.Changes) > 0}, nil
}
