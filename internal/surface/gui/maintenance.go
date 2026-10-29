package gui

import (
	"context"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Called only by Ready, after first paint. Failed/busy passes wait for the next
// idle opportunity; the durable completion time limits successful passes to weekly.
func (s *API) startMaintenance() {
	s.maintenanceOnce.Do(func() {
		s.autoWG.Add(1)
		go func() {
			defer s.autoWG.Done()
			timer := time.NewTimer(time.Minute)
			defer timer.Stop()
			for {
				select {
				case <-s.branchCtx.Done():
					return
				case <-timer.C:
					s.maintenanceCycle(s.branchCtx)
					timer.Reset(30 * time.Minute)
				}
			}
		}()
	})
}

// Never wait on a page lock, or keep it while opening/querying the database.
func (s *API) maintenanceSnapshot() (*app.App, bool) {
	// These workers also run outside Tasks. TryLock avoids reversing their
	// normal reader -> API lock order or waiting for a page to finish.
	if !s.costMeasure.mu.TryLock() {
		return nil, false
	}
	loading := s.costMeasure.page.Loading
	s.costMeasure.mu.Unlock()
	if loading {
		return nil, false
	}
	if !s.texMeasure.mu.TryLock() {
		return nil, false
	}
	loading = s.texMeasure.page != nil && s.texMeasure.page.Loading
	s.texMeasure.mu.Unlock()
	if loading {
		return nil, false
	}
	if !s.environment.gate.TryLock() {
		return nil, false
	}
	s.environment.gate.Unlock()
	if !s.filesMu.TryLock() {
		return nil, false
	}
	s.filesMu.Unlock()
	if !s.mu.TryLock() {
		return nil, false
	}
	defer s.mu.Unlock()
	if s.a.Uninstalled || s.needsClosed || s.needsRefreshing {
		return nil, false
	}
	for _, busy := range s.busy {
		if busy {
			return nil, false
		}
	}
	for _, job := range s.bgRead.jobs {
		if !job.done {
			return nil, false
		}
	}
	for _, task := range s.tasks.List() {
		if !task.Ended() {
			return nil, false
		}
	}
	r := &app.App{Store: &store.Store{Path: s.a.Store.Path}}
	r.ReadOptions(s.a)
	return r, true
}

func (s *API) maintenanceCycle(parent context.Context) {
	snapshot, ok := s.maintenanceSnapshot()
	if !ok || parent.Err() != nil {
		return
	}
	defer s.free.soon()
	db, err := store.OpenSession(snapshot.Store.Path)
	if err != nil {
		return
	}
	defer db.Close()
	snapshot.Store = db
	// Capture game roots once; the monitor must not query a connection currently
	// occupied by a maintenance transaction. Each probe refreshes the process list.
	envs, err := db.Envs()
	if err != nil {
		return
	}
	noGame := func() bool {
		return snapshot.MaintenanceGamesIdle(envs)
	}
	if !noGame() {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, idle := s.maintenanceSnapshot(); !idle || !noGame() {
					cancel()
					return
				}
			}
		}
	}()
	_, _ = snapshot.Maintain(ctx, true, false)
}
