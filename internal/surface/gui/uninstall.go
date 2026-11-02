package gui

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// settings › uninstall RimArcho (06 §2.14): the dry run for the player's answers, then the run with
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
	return s.startUninstall(c.Options())
}

func (s *API) startUninstall(options app.UninstallOptions) (err error) {
	s.uninstall.mu.Lock()
	if s.uninstall.status.Running || s.uninstall.closing {
		s.uninstall.mu.Unlock()
		return errors.New(view.T("uninstalling; you can close RimArcho when it finishes"))
	}
	s.uninstall.status = UninstallStatus{Running: true}
	s.uninstall.mu.Unlock()
	started := false
	defer func() {
		if !started {
			s.uninstall.mu.Lock()
			status := UninstallStatus{Done: true}
			if err != nil {
				status.Error = err.Error()
			}
			s.uninstall.status = status
			s.uninstall.mu.Unlock()
			s.send("uninstall", status)
		}
	}()
	release, err := s.beginAccount()
	if err != nil {
		return err
	}
	a, err := s.detachedReader()
	if err != nil {
		release()
		return err
	}
	p, err := a.CheckUninstall(options)
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
	s.uninstall.mu.Lock()
	for _, step := range p.Steps {
		s.uninstall.status.Steps = append(s.uninstall.status.Steps, UninstallProgressStep{Key: step.Key, Title: view.UninstallTitle(step.Key)})
	}
	s.uninstall.mu.Unlock()
	if s.uninstall.shutdownBlock != nil {
		s.uninstall.shutdownBlock(true)
	}
	started = true
	go func() {
		s.accountMu.Lock()
		s.ll = nil
		s.llFiles = nil
		s.accountMu.Unlock()
		p, err := a.Uninstall(options, func(pr app.UninstallProgress) {
			s.uninstall.mu.Lock()
			s.uninstall.status.I, s.uninstall.status.N = pr.I, pr.N
			s.uninstall.status.Step, s.uninstall.status.Item = view.UninstallTitle(pr.Step), pr.Item
			s.uninstall.status.StepKey = pr.Step
			status := s.uninstall.status
			s.uninstall.mu.Unlock()
			s.send("uninstall", status)
		})
		status := UninstallStatus{Done: true}
		if p != nil {
			status.Page = view.UninstallView(p, true)
		}
		if err != nil {
			status.Error = err.Error()
			if status.Page != nil {
				status.Page.Failed = max(1, status.Page.Failed)
				status.Page.Result = status.Error
			}
		}
		s.mu.Lock()
		s.a.Uninstalled = a.Uninstalled
		s.board, s.needs, s.lib = nil, nil, nil
		s.mu.Unlock()
		a.Close()
		release()
		if s.uninstall.shutdownBlock != nil {
			s.uninstall.shutdownBlock(false)
		}
		s.uninstall.mu.Lock()
		s.uninstall.status = status
		s.uninstall.mu.Unlock()
		s.send("uninstall", status)
	}()
	return nil
}

// Quit closes the window (after an uninstall); nothing in the development harness.
func (s *API) Quit() bool {
	if s.quit == nil || !s.allowClose() {
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
		return nil, errors.New("RimArcho was uninstalled; previews are disabled")
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

// The guard owns no App lock: even a slow filesystem operation must not stall a
// native close request. The status also survives WebView reloads (06 §2.14).
type uninstallState struct {
	mu            sync.Mutex
	status        UninstallStatus
	closing       bool
	shutdownBlock func(bool)
}

type UninstallProgressStep struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

type UninstallStatus struct {
	Steps   []UninstallProgressStep `json:"steps,omitempty"`
	StepKey string                  `json:"stepKey,omitempty"`
	Running bool                    `json:"running"`
	Done    bool                    `json:"done"`
	I       int                     `json:"i"`
	N       int                     `json:"n"`
	Step    string                  `json:"step"`
	Item    string                  `json:"item"`
	Page    *view.UninstallPage     `json:"page,omitempty"`
	Error   string                  `json:"error,omitempty"`
}

func (s *API) UninstallStatus() UninstallStatus {
	s.uninstall.mu.Lock()
	defer s.uninstall.mu.Unlock()
	return s.uninstall.status
}

func (s *API) allowClose() bool {
	s.uninstall.mu.Lock()
	running := s.uninstall.status.Running
	if !running {
		s.uninstall.closing = true
	}
	s.uninstall.mu.Unlock()
	if running {
		s.send("uninstall-close-blocked", nil)
	}
	return !running
}

func (s *API) guardWindowClose(event *application.WindowEvent) {
	if !s.allowClose() {
		event.Cancel()
	}
}
