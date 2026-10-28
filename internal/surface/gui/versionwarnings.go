package gui

import "github.com/ToumanLin/RimArcho/internal/view"

// VersionWarnings reads the current list only when its temporary panel is opened.
func (s *API) VersionWarnings(mm string, muted bool) (*view.VersionWarnings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.ReadVersionWarnings(s.a, mm, muted)
}
