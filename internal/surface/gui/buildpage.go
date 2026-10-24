package gui

import (
	"context"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

type buildPageJob struct {
	page *view.BuildPage
	err  error
	done bool
}

type buildPageReader struct {
	mu      sync.Mutex
	seq     uint64
	job     *buildPageJob
	workers sync.WaitGroup
}

// BuildPage starts a private reader and returns its ticket before measuring folders
// (06 §2.6, 06 §3.1). Only the latest request may publish a result.
func (s *API) BuildPage(tex int) (uint64, error) {
	r, err := s.detachedReader()
	if err != nil {
		return 0, err
	}
	b := &s.buildRead
	b.mu.Lock()
	b.seq++
	id := b.seq
	job := &buildPageJob{}
	b.job = job
	b.workers.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.workers.Done()
		defer r.Close()
		defer s.free.soon()
		p, err := measureBuildPage(r, tex)
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.job == job {
			job.page, job.err, job.done = p, err, true
		}
	}()
	return id, nil
}

// BuildPageResult returns nil while measuring; superseded tickets are invalid (06 §3.1).
func (s *API) BuildPageResult(id uint64) (*view.BuildPage, error) {
	b := &s.buildRead
	b.mu.Lock()
	defer b.mu.Unlock()
	if id != b.seq || b.job == nil {
		return nil, context.Canceled
	}
	if !b.job.done {
		return nil, nil
	}
	return b.job.page, b.job.err
}

func measureBuildPage(r *app.App, tex int) (*view.BuildPage, error) {
	st, err := r.Loadout()
	if err != nil {
		return nil, err
	}
	plan, chip := st.Plan.Name, view.EnvChip(st.Ix.Env.Name, st.Ix.Env.GameVersion)
	name := view.DefaultPackName(r, plan)
	pl, err := r.PlanPack(name)
	if err == nil {
		view.PlanTextures(r, pl, tex)
	}
	dir := r.PacksDir()
	if err != nil {
		return nil, err
	}
	pl.Measure(dir)
	p := view.BuildPageOf(pl, plan, chip, dir, tex)
	p.Name = name
	if t := r.TextureTool(); t != nil && t.Err == "" {
		p.ToolOK = true
	}
	return p, nil
}
