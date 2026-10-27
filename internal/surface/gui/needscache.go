package gui

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// All entries and worker state are guarded by API.mu. Published models are immutable.
type needsSnapshot struct {
	scope, key, index, list string
	directories             string
	ps                      *app.ProblemSet
	base                    *app.ProblemSet
	n                       *view.Needs
	eager                   bool
}

func sameNeedsList(a, b *app.LoadoutState) bool {
	return a != nil && b != nil && a.Ix.Env.ID == b.Ix.Env.ID && a.Plan.ID == b.Plan.ID && a.GameMM() == b.GameMM() && app.ListFingerprint(a.List) == app.ListFingerprint(b.List)
}
func fileNeedsStamp(path string) string {
	if path == "" {
		return ""
	}
	f, err := os.Stat(path)
	if err != nil {
		return path + ":" + err.Error()
	}
	return fmt.Sprintf("%s:%d:%d", path, f.Size(), f.ModTime().UnixNano())
}
func (s *API) needsKeyLocked() (scope, key, index, list string) {
	st, _ := s.a.Loadout()
	if st == nil {
		return
	}
	scope = fmt.Sprintf("%s|%d|%s|%s", st.Ix.Env.ID, st.Plan.ID, st.GameMM(), view.Lang())
	index = s.a.Store.IndexKey()
	list = app.ListFingerprint(st.List)
	cur, prev := s.a.LogPaths(st)
	// Links are outside SQLite. Probe them even on a hit; a removed junction target
	// must not hide behind a scan or database version.
	links, err := s.a.InspectGameLinks()
	b, _ := json.Marshal(links)
	lh := sha256.Sum256(b)
	dirs, _ := s.a.ManagedDirectories()
	db, _ := json.Marshal(dirs)
	dh := sha256.Sum256(db)
	key = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%x|%v", scope, s.a.Store.ChangeKey(), list, s.a.AnalysisKey(st, ""), fileNeedsStamp(cur), fileNeedsStamp(prev), fileNeedsStamp(s.a.ProblemLog(st)), lh, err) + fmt.Sprintf("|%x", dh)
	return
}
func (s *API) cachedNeedsLocked() (*needsSnapshot, string) {
	scope, key, _, _ := s.needsKeyLocked()
	return s.needsCache[scope], key
}
func (s *API) saveNeedsLocked(ps *app.ProblemSet, n *view.Needs, eager bool) *needsSnapshot {
	if ps == nil {
		return nil
	}
	*n = *view.RebindNeedsLinks(s.a, n, ps.GameLinks)
	scope, key, index, list := s.needsKeyLocked()
	if scope == "" {
		return nil
	}
	h := sha256.Sum256([]byte(key))
	n.Revision = fmt.Sprintf("%x", h[:12])
	s.needsSequence++
	n.Sequence = s.needsSequence
	n.Environment = ps.State.Ix.Env.ID
	n.Plan = ps.State.Plan.ID
	n.ListVersion = list
	n.Updating = n.Stale || n.Never || !eager
	dirs, _ := s.a.ManagedDirectories()
	db, _ := json.Marshal(dirs)
	c := &needsSnapshot{directories: string(db), scope: scope, key: key, index: index, list: list, ps: ps, base: ps, n: n, eager: eager}
	if s.needsCache == nil {
		s.needsCache = map[string]*needsSnapshot{}
	}
	if len(s.needsCache) >= 8 && s.needsCache[scope] == nil {
		for k := range s.needsCache {
			delete(s.needsCache, k)
			break
		}
	}
	s.needsCache[scope] = c
	s.needs, s.needsAt = n, key
	if w := s.workspace; w != nil && w.state == s.inventoryKey() && w.result.Board != nil && sameNeedsList(w.result.Board.St, ps.State) {
		old := *w.result
		board := *old.Board
		board.Problems = view.ProblemsOf(n)
		board.Status = n.Status
		board.Reasons = n.Reasons
		board.Needs = n.Count
		old.Board = &board
		s.workspace = &workspaceSnapshot{key: key, state: w.state, result: &old}
	}
	return c
}

// ListProblems supplies editor cards and per-mod counts without a second calculation.
func (s *API) ListProblems() *view.ListProblems {
	return view.ProblemsOf(s.Needs())
}
func (s *API) editProblemsLocked() *view.ListProblems {
	s.needsWanted = true
	scope, _, index, list := s.needsKeyLocked()
	previous := s.needsCache[scope]
	if previous != nil && previous.index == index {
		if previous.list == list {
			return view.ProblemsOf(s.needsLocked())
		}
		base := previous.base
		if base == nil {
			base = previous.ps
		}
		ps, err := s.a.IncrementalProblems(base)
		if err == nil {
			n := view.BuildNeeds(s.a, ps.State, ps)
			c := s.saveNeedsLocked(ps, n, false)
			c.base = base
			s.refreshNeedsLocked(c)
			return view.ProblemsOf(n)
		}
	}
	n := s.needsLocked()
	if c := s.needsCache[scope]; c != nil {
		s.refreshNeedsLocked(c)
	}
	return view.ProblemsOf(n)
}

// refreshNeedsLocked coalesces work on an independent session. Publication compares the
// input key again, so a slow log/analysis cannot overwrite a newer edit or environment.
func (s *API) refreshNeedsLocked(c *needsSnapshot) {
	if c == nil || s.needsClosed || s.needsRefreshing || s.tasks == nil {
		return
	}
	if c.eager {
		if _, key := s.cachedNeedsLocked(); key == c.key {
			return
		}
	}
	expected := s.inventoryKey()
	_, diskKey := s.cachedNeedsLocked()
	r, err := s.readInventorySession()
	if err != nil {
		return
	}
	s.needsRefreshing = true
	s.needsJobs.Add(1)
	go func() {
		defer s.needsJobs.Done()
		defer r.Close()
		r.LazyLog, r.LogCacheOnly = false, false
		ps, err := r.Problems(false, nil)
		var n *view.Needs
		if err == nil && ps != nil && ps.State != nil {
			n = view.BuildNeeds(r, ps.State, ps)
		}
		s.mu.Lock()
		s.needsRefreshing = false
		if s.needsClosed {
			s.mu.Unlock()
			return
		}
		_, nowKey := s.cachedNeedsLocked()
		if s.inventoryKey() != expected || nowKey != diskKey {
			if current, _ := s.cachedNeedsLocked(); current != nil {
				s.refreshNeedsLocked(current)
			}
			s.mu.Unlock()
			return
		}
		var event *view.ListProblems
		if n != nil {
			s.saveNeedsLocked(ps, n, true)
			event = view.ProblemsOf(n)
			if s.needsWanted && (n.Stale || n.Never) && !s.busy["analyze"] {
				s.startAnalyze()
			}
		} else {
			copy := *c.n
			copy.Updating = false
			copy.Error = fmt.Sprint(err)
			s.needsSequence++
			copy.Sequence = s.needsSequence
			c.n = &copy
			c.key = nowKey
			c.eager = true
			event = view.ProblemsOf(&copy)
		}
		s.mu.Unlock()
		s.send("list-problems", event)
	}()
}
