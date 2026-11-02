package gui

import (
	"context"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
	"sync"
)

// One worker per window; replacements wait for the cancelled reader to close.
type fileSearch struct {
	mu      sync.Mutex
	request uint64
	ctx     context.Context
	cancel  context.CancelFunc
	run     *view.FileSearchRun
	done    chan struct{}
	closed  bool
	workers sync.WaitGroup
}

func (s *API) fileSearchLibrary() (*app.Library, error) {
	r, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return r.FileSearchLibrary()
}

func (s *API) FileSearchInfo() (*view.FileSearchInfo, error) {
	lib, err := s.fileSearchLibrary()
	if err != nil {
		return nil, err
	}
	return view.BuildFileSearchInfo(lib, "", true)
}
func (s *API) FileSearchFilters(query string) (*view.FileSearchInfo, error) {
	lib, err := s.fileSearchLibrary()
	if err != nil {
		return nil, err
	}
	return view.BuildFileSearchInfo(lib, query, false)
}
func (s *API) BeginFileSearch() uint64 {
	w := &s.search
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
	}
	w.request++
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.run = nil
	return w.request
}
func (s *API) StartFileSearch(q view.FileSearchReq, request uint64) (uint64, error) {
	w := &s.search
	w.mu.Lock()
	if w.closed || request != w.request || w.ctx == nil || w.ctx.Err() != nil || w.run != nil {
		w.mu.Unlock()
		return 0, context.Canceled
	}
	ctx, run, previous := w.ctx, view.NewFileSearchRun(), w.done
	done := make(chan struct{})
	w.run, w.done = run, done
	w.workers.Add(1)
	w.mu.Unlock()
	go func() {
		defer w.workers.Done()
		defer close(done)
		defer s.free.soon()
		if previous != nil {
			<-previous
		}
		if ctx.Err() != nil {
			run.Search(ctx, nil, q)
			return
		}
		r, err := s.detachedReader()
		if err != nil {
			run.Fail(err)
			return
		}
		defer r.Close()
		run.Search(ctx, r, q)
	}()
	return request, nil
}
func (s *API) FileSearchUpdate(request uint64, since int) (view.FileSearchUpdate, error) {
	w := &s.search
	w.mu.Lock()
	run := w.run
	valid := request == w.request && !w.closed
	w.mu.Unlock()
	if !valid || run == nil {
		return view.FileSearchUpdate{}, context.Canceled
	}
	return run.Update(since), nil
}
func (s *API) StopFileSearch(request uint64) {
	w := &s.search
	w.mu.Lock()
	defer w.mu.Unlock()
	if request == w.request && w.cancel != nil {
		w.cancel()
	}
}

// ModCandidates also serves sorting-rule name inputs.
func (s *API) ModCandidates(text string) ([]view.RuleMod, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.RuleCandidates(s.a, text, "")
}
func (s *API) stopFileSearch() {
	w := &s.search
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
	}
	w.request++
	w.run = nil
}
func (s *API) closeFileSearch() {
	w := &s.search
	w.mu.Lock()
	w.closed = true
	if w.cancel != nil {
		w.cancel()
	}
	w.run = nil
	w.mu.Unlock()
	w.workers.Wait()
}
func (s *API) ReadSearchFile(id, path string, q view.FileSearchReq) (*view.SearchFileText, error) {
	s.mu.Lock()
	ix, err := s.a.Index()
	root := s.a.SourceDir()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return view.ReadSearchFile(ix, root, id, path, q)
}
func (s *API) OpenSearchFile(id, path string) error {
	s.mu.Lock()
	ix, err := s.a.Index()
	root := s.a.SourceDir()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if _, err := view.SearchFolder(ix, root, id, path); err != nil {
		return err
	}
	if s.openDir != nil {
		return s.openDir(path)
	}
	return openDir(path)
}
func (s *API) OpenSearchFolder(id, path string) error {
	s.mu.Lock()
	ix, err := s.a.Index()
	root := s.a.SourceDir()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	dir, err := view.SearchFolder(ix, root, id, path)
	if err != nil {
		return err
	}
	if s.openDir != nil {
		return s.openDir(dir)
	}
	return openDir(dir)
}
