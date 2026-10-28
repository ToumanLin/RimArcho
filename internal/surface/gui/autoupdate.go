package gui

import (
	"context"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// One timer, started only after first paint. Detached sessions are closed after
// every pass so a sleeping scheduler retains neither inventory nor data indexes.
func (s *API) startAutomatic() {
	s.autoOnce.Do(func() {
		s.autoWG.Add(1)
		go func() {
			defer s.autoWG.Done()
			app.AutomaticLoop(s.branchCtx, s.automaticCycle)
		}()
	})
}

func (s *API) automaticCycle(parent context.Context) {
	if parent.Err() != nil {
		return
	}
	r, err := s.detachedReader()
	if err != nil {
		return
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	r.RunAutomaticCycle(ctx, func(kind string, run func() error) {
		if ctx.Err() != nil {
			return
		}
		// Yield to explicit work instead of checking metadata while files are replaced.
		for _, t := range s.tasks.List() {
			if t.State == app.TaskRunning || t.State == app.TaskQueued {
				return
			}
		}
		var release func()
		if kind == "session" || kind == "mods" {
			var e error
			release, e = s.beginAccount()
			if e != nil {
				return
			}
			defer release()
		}
		before := r.AutomaticStatus(kind)
		id := s.tasks.Begin(view.TaskAutomatic, view.AutomaticName(kind), nil)
		checkErr := run()
		after := r.AutomaticStatus(kind)
		state, result := app.TaskDone, view.Text(after.State)
		if checkErr != nil {
			state = app.TaskFailed
		}
		// Automatic failures remain inspectable without red badges or failure notices.
		s.tasks.Update(id, func(t *app.Task) { t.Seen = true; t.Target = kind })
		s.tasks.Finish(id, state, result)
		s.mu.Lock()
		s.lib, s.board, s.needs = nil, nil, nil
		if kind == "data" {
			s.a.DropCommunityData()
		}
		s.mu.Unlock()
		if kind == "session" || kind == "mods" {
			s.ll = nil
		}
		s.subRevision.Add(1)
		if parent.Err() == nil {
			s.send("automatic-update", map[string]any{"kind": kind, "notice": view.AutomaticNotice(before, after)})
		}
	})
}
