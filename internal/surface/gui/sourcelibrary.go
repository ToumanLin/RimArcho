package gui

import (
	"context"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// SourceUnused is what "remove unused" would remove: how many folders and their size.
type SourceUnused struct {
	Folders int    `json:"folders"`
	Size    string `json:"size"`
}

// SourceLibraryUnused reads sizes and liveness only after the cleanup action.
func (s *API) SourceLibraryUnused() (*SourceUnused, error) {
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	p, err := a.CleanSourceLibrary(context.Background(), false)
	if err != nil {
		return nil, err
	}
	return &SourceUnused{Folders: p.Folders, Size: view.SizeText(p.Bytes)}, nil
}
