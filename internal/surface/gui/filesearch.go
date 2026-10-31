package gui

import (
	"context"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// 搜索文件 (06 §2.9): one search at a time. A newer search or 停止 cancels the one before,
// whose answer is then dropped (06 §3.1 rule 9). The search runs in Wails' calling goroutine
// on a private read session: neither the editor's lock nor a page reader waits for it.
type fileSearch struct {
	mu       sync.Mutex
	request  uint64
	cancel   context.CancelFunc
	progress string
	closed   bool
	workers  sync.WaitGroup
}

// FileSearchInfo is what the page shows before a search: the default scope in words.
func (s *API) FileSearchInfo() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.FileSearchScope(s.a)
}

// FileSearch runs a search; request numbers it (BeginFileSearch).
func (s *API) FileSearch(q view.FileSearchReq, request uint64) (*view.FileSearchPage, error) {
	w := &s.search
	w.mu.Lock()
	if w.closed || request != w.request {
		w.mu.Unlock()
		return nil, context.Canceled
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel, w.progress = cancel, ""
	w.workers.Add(1)
	w.mu.Unlock()
	defer w.workers.Done()
	defer cancel()
	defer s.free.soon()
	r, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	page, err := view.FileSearch(ctx, r, q, func(p string) {
		w.mu.Lock()
		if request == w.request {
			w.progress = p
		}
		w.mu.Unlock()
	})
	if ctx.Err() != nil {
		return nil, context.Canceled
	}
	return page, err
}

// BeginFileSearch cancels the search before and numbers the next one.
func (s *API) BeginFileSearch() uint64 {
	w := &s.search
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.request++
	w.progress = ""
	return w.request
}

// StopFileSearch is 停止 (and leaving the page): the search request ends.
func (s *API) StopFileSearch(request uint64) {
	w := &s.search
	w.mu.Lock()
	defer w.mu.Unlock()
	if request == w.request && w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}

// FileSearchProgress is the decompiling's progress of search request, "" when none.
func (s *API) FileSearchProgress(request uint64) string {
	w := &s.search
	w.mu.Lock()
	defer w.mu.Unlock()
	if request != w.request {
		return ""
	}
	return w.progress
}

// ModCandidates are mods to add to the search's scope while typing their name.
func (s *API) ModCandidates(text string) ([]view.RuleMod, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.RuleCandidates(s.a, text, "")
}

// stopFileSearch: the player left 搜索文件; its search is no longer wanted.
func (s *API) stopFileSearch() {
	w := &s.search
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}

func (s *API) closeFileSearch() {
	w := &s.search
	w.mu.Lock()
	w.closed = true
	if w.cancel != nil {
		w.cancel()
	}
	w.mu.Unlock()
	w.workers.Wait()
}

// OpenSearchFolder opens the folder holding a result's file (ctrl+o on a result): only a
// file inside mod id's folder, or a decompiled file in RimArcho's C# cache.
func (s *API) OpenSearchFolder(id, path string) error {
	s.mu.Lock()
	ix, err := s.a.Index()
	home := s.a.Home
	s.mu.Unlock()
	if err != nil {
		return err
	}
	dir, err := view.SearchFolder(ix, home, id, path)
	if err != nil {
		return err
	}
	if s.openDir != nil {
		return s.openDir(dir)
	}
	return openDir(dir)
}
