package gui

import "github.com/ToumanLin/RimArcho/internal/view"

// HomeMemory refreshes only stored totals and the OS snapshot, never Mod files.
func (s *API) HomeMemory() (*view.MemoryLaunch, error) {
	r, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	r.WorkspaceSnapshot()
	r.OpenPlan, r.PlanName = 0, ""
	st, err := r.Loadout()
	if err != nil {
		return nil, err
	}
	return view.BuildMemoryLaunch(r, st), nil
}
