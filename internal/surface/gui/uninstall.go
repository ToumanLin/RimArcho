package gui

import (
	"errors"
	"fmt"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 设置 › 卸载 RimArcho (06 §2.14): the dry run for the player's answers, then the run with
// "uninstall" events and the report.

// UninstallPlan is the plan for these answers (nothing changes).
func (s *API) UninstallPlan(c view.UninstallChoice) (*view.UninstallPage, error) {
	s.uninstallRead.mu.Lock()
	defer s.uninstallRead.mu.Unlock()
	r, err := s.uninstallSession()
	if err != nil {
		return nil, err
	}
	defer s.free.soon()
	p, err := r.PlanUninstallPreview(c.Options(), &s.uninstallSizes)
	if err != nil {
		return nil, err
	}
	return view.UninstallView(p, false), nil
}

// UninstallRun runs it in the background; "uninstall" events carry {i, n, step, item} and at
// the end {done, page} or {done, error}. Work owns a detached application session.
func (s *API) UninstallRun(c view.UninstallChoice) error {
	release, err := s.beginAccount()
	if err != nil {
		return err
	}
	a, err := s.detachedReader()
	if err != nil {
		release()
		return err
	}
	p, err := a.CheckUninstall(c.Options())
	if err == nil {
		err = p.Blocked()
	}
	if err != nil {
		a.Close()
		release()
		var bl = app.ErrUninstallBlocked
		if errors.Is(err, bl) {
			return fmt.Errorf("%s", view.UninstallView(p, false).Blocked)
		}
		return err
	}
	go func() {
		defer release()
		defer a.Close()
		s.ll = nil
		s.llFiles = nil
		p, err := a.Uninstall(c.Options(), func(pr app.UninstallProgress) {
			s.send("uninstall", map[string]any{"i": pr.I, "n": pr.N, "step": view.UninstallTitle(pr.Step), "item": pr.Item})
		})
		ev := map[string]any{"done": true}
		if err != nil {
			ev["error"] = err.Error()
		} else {
			ev["page"] = view.UninstallView(p, true)
		}
		s.mu.Lock()
		s.a.Uninstalled = a.Uninstalled
		s.board, s.needs, s.lib = nil, nil, nil
		s.mu.Unlock()
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

// Called with uninstallRead.mu held. Only invocation options are copied under mu;
// the reader's database-keyed input caches survive, but no plan is retained.
func (s *API) uninstallSession() (*app.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.a.Uninstalled {
		return nil, errors.New("RimArcho 已卸载，预览已停用")
	}
	if s.uninstallRead.a == nil {
		r, err := s.a.ReadSession()
		if err != nil {
			return nil, err
		}
		s.uninstallRead.a = r
	}
	r := s.uninstallRead.a
	r.ReadOptions(s.a)
	if e, err := s.a.Env(); err == nil {
		r.EnvName = e.Name
	}
	r.AnalyzeExe, r.AnalyzeWait = s.Exe, 200*time.Millisecond
	return r, nil
}
