package gui

import (
	"errors"
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 设置 › 卸载 RimArcho (06 §2.14): the dry run for the player's answers, then the run with
// "uninstall" events and the report.

// UninstallPlan is the plan for these answers (nothing changes).
func (s *API) UninstallPlan(c view.UninstallChoice) (*view.UninstallPage, error) {
	r, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	defer s.free.soon()
	p, err := r.PlanUninstall(c.Options())
	if err != nil {
		return nil, err
	}
	return view.UninstallView(p, false), nil
}

// UninstallRun runs it in the background; "uninstall" events carry {i, n, step, item} and at
// the end {done, page} or {done, error}. Every other call waits until it has finished.
func (s *API) UninstallRun(c view.UninstallChoice) error {
	s.mu.Lock()
	p, err := s.a.PlanUninstall(c.Options())
	if err == nil {
		err = p.Blocked()
	}
	if err != nil {
		s.mu.Unlock()
		var bl = app.ErrUninstallBlocked
		if errors.Is(err, bl) {
			return fmt.Errorf("%s", view.UninstallView(p, false).Blocked)
		}
		return err
	}
	go func() {
		defer s.mu.Unlock()
		p, err := s.a.Uninstall(c.Options(), func(pr app.UninstallProgress) {
			s.send("uninstall", map[string]any{"i": pr.I, "n": pr.N, "step": view.UninstallTitle(pr.Step), "item": pr.Item})
		})
		ev := map[string]any{"done": true}
		if err != nil {
			ev["error"] = err.Error()
		} else {
			ev["page"] = view.UninstallView(p, true)
		}
		s.board, s.needs = nil, nil
		s.send("uninstall", ev)
	}()
	return nil
}

// Quit closes the window (after an uninstall); nothing in the development harness.
func (s *API) Quit() bool {
	if s.quit == nil {
		return false
	}
	go s.quit()
	return true
}
