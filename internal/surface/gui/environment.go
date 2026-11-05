package gui

import (
	"errors"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/perftrace"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Environment preparation owns a separate session and never holds the page mutex
// during disk reads, linking or scanning (16 §3.1, 16 §8.4).
type environmentWorker struct {
	mu      sync.Mutex
	gate    sync.Mutex
	wg      sync.WaitGroup
	readSeq uint64
	seq     uint64
	closed  bool
}

type workspaceSnapshot struct {
	key    string
	state  string
	result *EnvironmentResult
}

type EnvironmentResult struct {
	Request uint64          `json:"request"`
	Stale   bool            `json:"stale,omitempty"`
	Error   string          `json:"error,omitempty"`
	Text    string          `json:"text,omitempty"`
	Names   []string        `json:"names,omitempty"`
	Board   *Board          `json:"board,omitempty"`
	Shell   *Shell          `json:"shell,omitempty"`
	History []view.HistItem `json:"history,omitempty"`
}

// UseEnv returns immediately; environment events carry the prepared game's data.
func (s *API) UseEnv(request uint64, name string) bool {
	return s.queueEnvironment(request, name, "", 0, "", nil)
}

// HomeApply applies the selected game/list in the background (16 §2.3).
func (s *API) HomeApply(request uint64, kind, name string, plan int64, pack string) bool {
	return s.queueEnvironment(request, name, kind, plan, pack, nil)
}

// GuideAdd registers games in the background; GuideScan owns their first scan.
func (s *API) GuideAdd(request uint64, games []view.GameFound) bool {
	return s.queueEnvironment(request, "", "add", 0, "", games)
}

func (s *API) queueEnvironment(request uint64, name, kind string, plan int64, pack string, games []view.GameFound) bool {
	w := &s.environment
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	if request <= w.seq {
		go s.send("environment", &EnvironmentResult{Request: request, Stale: true})
		return true
	}
	w.seq = request
	seq := request
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		// Serialise destination writes; superseded queued requests do no work.
		w.gate.Lock()
		defer w.gate.Unlock()
		result := &EnvironmentResult{Request: request}
		current := func() bool {
			w.mu.Lock()
			defer w.mu.Unlock()
			return !w.closed && w.seq == seq
		}
		defer func() {
			result.Stale = !current()
			s.send("environment", result)
		}()
		if !current() {
			return
		}
		r, err := s.detachedReader()
		if err != nil {
			result.Error = err.Error()
			return
		}
		defer r.Close()
		if kind == "add" {
			result.Names, err = view.CreateGames(r, games)
			if err == nil && len(result.Names) > 0 {
				name = result.Names[0]
			}
		} else {
			_, err = r.PrepareEnv(name)
		}
		if err != nil {
			result.Error = view.Humanize(nil, err.Error())
			return
		}
		if name == "" {
			return
		}
		r.EnvName = name
		if !current() {
			return
		}
		if kind == view.RowPlan {
			_, result.Text, err = view.UsePlan(r, plan)
		} else {
			result.Text = view.Tf("switched to %s", name)
		}
		if err != nil {
			result.Error = err.Error()
			return
		}
		// Registration deliberately skips the board: it would import/load thousands
		// of mods before GuideScan even starts (16 §8.4).
		var reader *API
		if kind != "add" {
			reader = &API{a: r, busy: map[string]bool{}}
			result.Board = reader.boardLocked()
			if result.Board.Error != "" {
				result.Error = result.Board.Error
				return
			}
			result.Shell = reader.shellLocked(reader.needs)
			if result.Shell.Error != "" {
				result.Error = result.Shell.Error
				return
			}
			result.History, err = reader.History()
			if err != nil {
				result.Error = err.Error()
				return
			}
		}
		defer s.releaseSubscriptionsForEnvironment()
		s.mu.Lock()
		defer s.mu.Unlock()
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.closed || w.seq != seq {
			return
		}
		e, err := r.Env()
		if err == nil && e == nil {
			err = errors.New("no game")
		}
		if err == nil {
			err = s.a.Store.SetCurrentEnv(e.ID)
		}
		if err == nil && (kind == view.RowPlan || kind == view.RowPack) {
			featured := ""
			if kind == view.RowPack {
				st := reader.board.St
				featured = view.FeaturedValue(name, pack, st.Plan.ID)
				result.Text = view.Tf("%s is ready to launch", pack)
			}
			err = s.a.Store.SetMeta(view.FeaturedKey, featured)
		}
		if err != nil {
			result.Error = err.Error()
			return
		}
		s.a.EnvName = name
		s.board, s.needs, s.lib = nil, nil, nil
		s.logLinks, s.logPath = nil, ""
		if reader != nil {
			s.a.AdoptReadState(r)
			s.board, s.boardAt = reader.board, reader.boardAt
			if c, _ := reader.cachedNeedsLocked(); c != nil {
				s.saveNeedsLocked(c.ps, c.n, c.eager)
				result.Board.Problems = view.ProblemsOf(c.n)
			}
			result.Board.Stamp = s.a.Store.ChangeKey()
			result.Board.Scanning, result.Board.Analyzing = s.busy["scan"], s.busy["analyze"]
		}
	}()
	return true
}

// Workspace reads the board/sidebar/history without blocking a GUI method or sharing
// the page mutex during index loads (16 §8.4). Results use the same event contract.
func (s *API) Workspace(request uint64) bool {
	w := &s.environment
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	if request <= w.readSeq {
		go s.send("environment", &EnvironmentResult{Request: request, Stale: true})
		return true
	}
	w.readSeq = request
	seq := w.seq
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		var result *EnvironmentResult
		for attempt := 0; attempt < 4; attempt++ {
			var retry bool
			result, retry = s.workspaceRead(request, seq)
			if !retry {
				break
			}
		}
		s.send("environment", result)
	}()
	return true
}

// Analysis/log commits may change the database while rows are read. Re-read those
// inputs on a fresh snapshot; an edit or navigation still rejects the original request.
func (s *API) workspaceRead(request, seq uint64) (*EnvironmentResult, bool) {
	defer perftrace.Span("Workspace-read")()
	w := &s.environment
	result := &EnvironmentResult{Request: request}
	s.workspaceReader.mu.Lock()
	defer s.workspaceReader.mu.Unlock()
	// Even a warm board can be expensive (all copies, needs and sidebar).
	// Capture a detached index under mu; render on its own session.
	s.mu.Lock()
	key := s.inventoryKey()
	edits := s.listEdits
	// Validating needs loads the loadout. Only try the fast path while the main
	// index is warm; a scan-invalidated index must load on the detached reader.
	if cached := s.workspace; cached != nil && cached.state == key && s.a.IndexCached() {
		if _, needsKey := s.cachedNeedsLocked(); cached.key == needsKey {
			copy := *cached.result
			copy.Request = request
			*result = copy
			board := *result.Board
			board.Scanning, board.Analyzing = s.busy["scan"], s.busy["analyze"]
			result.Board = &board
			w.mu.Lock()
			result.Stale = w.closed || w.seq != seq || w.readSeq != request
			w.mu.Unlock()
			s.mu.Unlock()
			return result, false
		}
	}
	r, err := s.inventorySession(&s.workspaceReader)
	s.mu.Unlock()
	if err == nil {
		err = s.refreshPageIndex(r)
	}
	if err != nil {
		result.Error = err.Error()
		return result, false
	}
	release, err := pinPageRead(r)
	if err != nil {
		result.Error = err.Error()
		return result, false
	}
	defer release()
	reader := &API{a: r, busy: map[string]bool{}, board: s.workspaceReader.board, boardAt: s.workspaceReader.boardAt}
	result.Board = reader.boardLocked()
	s.workspaceReader.board, s.workspaceReader.boardAt = reader.board, reader.boardAt
	if result.Board.Error != "" {
		result.Error = result.Board.Error
		return result, false
	}
	result.Shell = reader.shellLocked(reader.needs)
	if result.Shell.Error != "" {
		result.Error = result.Shell.Error
		return result, false
	}
	result.History, err = reader.History()
	if err != nil {
		result.Error = err.Error()
		return result, false
	}
	c, _ := reader.cachedNeedsLocked()
	release()
	s.mu.Lock()
	defer s.mu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	result.Stale = w.closed || w.seq != seq || w.readSeq != request || s.listEdits != edits
	if !result.Stale && s.inventoryKey() != key {
		result.Stale = true
		if !s.a.IndexCached() {
			return result, true
		}
		current, _ := s.a.Loadout()
		if sameNeedsList(current, result.Board.St) {
			return result, true
		}
	}
	if result.Stale {
		return result, false
	}
	// Adopt only if no edit or option change overtook this read.
	s.a.AdoptReadSnapshot(r)
	s.board, s.boardAt = reader.board, reader.boardAt
	result.Board.Stamp = s.a.Store.ChangeKey()
	result.Board.Scanning, result.Board.Analyzing = s.busy["scan"], s.busy["analyze"]
	if c != nil {
		s.saveNeedsLocked(c.ps, c.n, c.eager)
		result.Board.Problems = view.ProblemsOf(c.n)
	}
	_, cacheKey := s.cachedNeedsLocked()
	saved := *result
	s.workspace = &workspaceSnapshot{key: cacheKey, state: s.inventoryKey(), result: &saved}
	return result, false
}
