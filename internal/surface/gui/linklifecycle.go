package gui

import (
	"github.com/ToumanLin/RimArcho/internal/view"
	"strings"
)

// DirectoryMovePreview only reads the common incoming-link plan. The existing
// directory dialog displays these lines before its move button becomes active.
func (s *API) DirectoryMovePreview(id, path string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.a.PlanDirectoryMove(id, path)
	if err != nil {
		return nil, err
	}
	lines := p.Lines()
	if strings.HasPrefix(id, "game:") {
		e, err := s.a.Store.EnvByID(strings.TrimPrefix(id, "game:"))
		if err != nil {
			return nil, err
		}
		if e != nil {
			more, err := s.a.EnvGameChangePreview(e.Name, path)
			if err != nil {
				return nil, err
			}
			for _, line := range more {
				lines = append(lines, view.Text(line))
			}
		}
	}
	return lines, nil
}

func (s *API) PackRemovePreview(name string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.a.RemovePack(name, true)
	if err != nil {
		return nil, err
	}
	p, err := s.a.PlanIncomingLinks(dir, "")
	if err != nil {
		return nil, err
	}
	return p.Lines(), nil
}
