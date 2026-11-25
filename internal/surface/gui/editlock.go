//go:build windows || cgo

package gui

import (
	"errors"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

type editTarget struct {
	env, envName, planName string
	plan                   int64
}

const editDuringLaunch = "RimArcho is preparing a launch. Try editing the list again after the launch finishes."

func (s *API) editTargetLocked() editTarget {
	a := s.a
	target := editTarget{envName: a.EnvName, planName: a.PlanName, plan: a.OpenPlan}
	if e, err := a.Env(); err == nil {
		target.env = e.ID
		if target.plan == 0 && target.planName == "" {
			target.plan = a.Store.CurrentPlanID(e.ID)
		}
	}
	return target
}

// lockEdit waits for the OS lock before taking API.mu for the edit. The main
// App's implicit sync may try the OS lock once while holding API.mu. A single
// attempt there lets a waiting editor take API.mu without a lock cycle.
func (s *API) lockEdit() (func(), error) {
	s.mu.Lock()
	home, target := s.a.Home, s.editTargetLocked()
	s.mu.Unlock()
	guard, err := app.LockWorkspace(home, func() error {
		if s.launching.Load() {
			return errors.New(view.T(editDuringLaunch))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	closed := s.actionsClosed
	if closed || target != s.editTargetLocked() {
		guard.Release()
		s.mu.Unlock()
		if closed {
			return nil, errors.New(view.T("the window is closing"))
		}
		return nil, errors.New(view.T("the open list or Game Config changed. Review the list and try again."))
	}
	release := guard.Bind(s.a)
	return func() {
		release()
		s.mu.Unlock()
	}, nil
}
