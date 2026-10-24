package gui

// DirectoryMovePreview only reads the common incoming-link plan. The existing
// directory dialog displays these lines before its move button becomes active.
func (s *API) DirectoryMovePreview(id, path string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.a.PlanDirectoryMove(id, path)
	if err != nil {
		return nil, err
	}
	return p.Lines(), nil
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
