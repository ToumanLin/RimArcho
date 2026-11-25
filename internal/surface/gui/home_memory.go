//go:build windows || cgo

package gui

import "github.com/ToumanLin/RimArcho/internal/view"

// HomeMemory refreshes only stored totals and the OS snapshot, never Mod files. The launch
// page asks every 30 seconds (16 §2.16.1 G19), so it borrows the retained log session, as
// Home does: a warm index and list state make an unchanged refresh rebuild
// nothing. A fresh session here rebuilt the whole index and list each time (about 30 MB
// allocated per call for 6,574 Mods, 17 §7.3).
func (s *API) HomeMemory() (*view.MemoryLaunch, error) {
	return join(&s.flight, "HomeMemory", s.homeMemoryRead)
}

func (s *API) homeMemoryRead() (*view.MemoryLaunch, error) {
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	r, releaseSession, err := s.launchSession()
	defer releaseSession()
	if err == nil && !r.IndexCached() {
		err = s.refreshPageIndex(r)
	}
	if err != nil {
		return nil, err
	}
	// The current list, the one launched (07 §1.3), like Home.
	r.OpenPlan, r.PlanName = 0, ""
	st, err := r.Loadout()
	if err != nil {
		return nil, err
	}
	return view.BuildMemoryLaunch(r, st), nil
}
