//go:build windows || cgo

package gui

import (
	"errors"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

type logCleanupPreview struct {
	Path      string
	Token     string
	Size      string
	Omitted   string
	Remaining string
}

func (s *API) PreviewLogCleanup(path string, trim bool) (*logCleanupPreview, error) {
	s.mu.Lock()
	r, err := s.pageReader()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	p, err := r.PreviewLogCleanup(path, trim)
	if err != nil {
		return nil, cleanupError(err)
	}
	return &logCleanupPreview{p.Path, p.Token, view.SizeText(p.Size), view.SizeText(p.Omitted), view.SizeText(p.Remaining)}, nil
}

func (s *API) ApplyLogCleanup(path, token string, trim bool) error {
	s.mu.Lock()
	r, err := s.pageReader()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	defer r.Close()
	return cleanupError(r.ApplyLogCleanup(path, token, trim))
}

func cleanupError(err error) error {
	switch {
	case errors.Is(err, app.ErrGameRunning):
		return errors.New(view.T("Close the game before trimming or deleting a log."))
	case errors.Is(err, app.ErrLogCleanupChanged):
		return errors.New(view.T("The log changed. Review its new size and confirm again."))
	case errors.Is(err, app.ErrLogCleanupTarget):
		return errors.New(view.T("Choose an oversized Player.log or Player-prev.log for this game."))
	default:
		return err
	}
}
