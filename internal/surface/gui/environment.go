package gui

import (
	"errors"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// Environment preparation owns a separate session and never holds the page mutex
// during disk reads, linking or scanning (06 §3.1, 06 §8.4).
type environmentWorker struct {
	mu      sync.Mutex
	gate    sync.Mutex
	wg      sync.WaitGroup
	readSeq uint64
	seq     uint64
	closed  bool
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

// HomeApply applies the selected game/list in the background (06 §2.3).
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
		// of mods before GuideScan even starts (06 §8.4).
		var reader *API
		if kind != "add" {
			reader = &API{a: r, busy: map[string]bool{}}
			result.Board = reader.boardLocked()
			if result.Board.Error != "" {
				result.Error = result.Board.Error
				return
			}
			result.Shell = reader.Shell()
			result.History, err = reader.History()
			if err != nil {
				result.Error = err.Error()
				return
			}
		}
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
			s.board, s.needs, s.needsAt = reader.board, reader.needs, s.a.Store.ChangeKey()
			result.Board.Stamp = s.a.Store.ChangeKey()
			result.Board.Scanning, result.Board.Analyzing = s.busy["scan"], s.busy["analyze"]
		}
	}()
	return true
}

// Workspace reads the board/sidebar/history without blocking a GUI method or sharing
// the page mutex during index loads (06 §8.4). Results use the same event contract.
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
		result := &EnvironmentResult{Request: request}
		defer func() { s.send("environment", result) }()
		r, err := s.detachedReader()
		if err != nil {
			result.Error = err.Error()
			return
		}
		defer r.Close()
		reader := &API{a: r, busy: map[string]bool{}}
		result.Board = reader.boardLocked()
		result.Shell = reader.Shell()
		result.History, err = reader.History()
		if err != nil {
			result.Error = err.Error()
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		w.mu.Lock()
		defer w.mu.Unlock()
		result.Stale = w.closed || w.seq != seq || w.readSeq != request
		if result.Stale {
			return
		}
		s.a.AdoptReadState(r)
		s.board, s.needs, s.needsAt = reader.board, reader.needs, s.a.Store.ChangeKey()
		result.Board.Stamp = s.a.Store.ChangeKey()
		result.Board.Scanning, result.Board.Analyzing = s.busy["scan"], s.busy["analyze"]
	}()
	return true
}
