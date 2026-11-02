package app

import (
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// FileSearchLibrary reads stored inventory and lists, including Core and DLCs.
// It does not sync ModsConfig or probe Mod folders. Query matching is Library.Filter.
func (a *App) FileSearchLibrary() (*Library, error) {
	e, err := a.Env()
	if err != nil {
		return nil, err
	}
	// Legacy link adoption can resolve filesystem aliases; a page-opening snapshot
	// leaves that repair to normal inventory/scan operations.
	var ix *Index
	if a.IndexCached() {
		ix, err = a.Index()
	} else {
		ix, err = a.readIndex(e, false)
	}
	if err != nil {
		return nil, err
	}
	plan, err := a.planFor(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		plan = &store.Plan{Name: "current list"}
	}
	aliases, err := a.Store.AllAliases(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	all := Candidates(ix, aliases)
	reg := loadout.NewRegistry(all)
	st := &LoadoutState{Ix: ix, Plan: plan, Reg: reg, Aliases: aliases, cands: all, MM: a.GameVersion,
		localize: a.localizer(ix.Env.ID, reg, ix)}
	entries, err := a.planList(plan)
	if err != nil {
		return nil, err
	}
	st.List = st.readList(entries.Encode())
	return a.buildLibrary(st, "", true, true)
}

// FileSearchRunLibrary resolves the complete library metadata when a search is
// explicitly requested. Only installed copies are later searched by the view.
func (a *App) FileSearchRunLibrary() (*Library, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	return a.buildLibrary(st, "", true, false)
}
