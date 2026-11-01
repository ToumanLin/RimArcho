package gui

import (
	"context"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// SourceLibraryUnused reads sizes and liveness only after the cleanup action.
func (s *API) SourceLibraryUnused() (*app.SourceCleanPreview, error) {
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	return a.CleanSourceLibrary(context.Background(), false)
}
