package gui

import (
	"context"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/view"
)

type problemLookup struct {
	mu      sync.Mutex
	request uint64
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
	workers sync.WaitGroup
}

// BeginProblemProviders supersedes older lookups before the browser starts a read.
func (s *API) BeginProblemProviders() uint64 {
	w := &s.problemLookup
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
	}
	w.request++
	w.ctx, w.cancel = context.WithCancel(context.Background())
	if w.closed {
		w.cancel()
	}
	return w.request
}

func (s *API) CancelProblemProviders(request uint64) {
	w := &s.problemLookup
	w.mu.Lock()
	defer w.mu.Unlock()
	if request == w.request && w.cancel != nil {
		w.cancel()
	}
}

// ProblemProviders runs in Wails' calling goroutine with a private read session;
// neither the navigation lock nor the log reader is held during file IO.
func (s *API) ProblemProviders(ref string, request uint64) (*view.ProblemPanel, error) {
	w := &s.problemLookup
	w.mu.Lock()
	if w.closed || request != w.request || w.ctx == nil || w.ctx.Err() != nil {
		w.mu.Unlock()
		return nil, context.Canceled
	}
	ctx := w.ctx
	w.workers.Add(1)
	w.mu.Unlock()
	defer w.workers.Done()
	defer s.free.soon()
	r, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.LazyLog, r.LogCacheOnly = false, false
	ps, err := r.Problems(false, nil)
	if err != nil {
		return nil, err
	}
	return view.BuildProblemPanelContext(ctx, r, ps, ref)
}

func (s *API) closeProblemProviders() {
	w := &s.problemLookup
	w.mu.Lock()
	w.closed = true
	if w.cancel != nil {
		w.cancel()
	}
	w.mu.Unlock()
	w.workers.Wait()
}
