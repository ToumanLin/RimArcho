package gui

import "github.com/ToumanLin/RimArcho/internal/view"

// Tips returns usage hints in the selected interface language.
func (s *API) Tips() []view.Tip {
	return view.Tips()
}
