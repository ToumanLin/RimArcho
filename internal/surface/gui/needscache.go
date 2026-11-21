package gui

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"time"

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
	return s.needsKeyOfLocked(nil)
}

// needsKeyOfLocked is needsKeyLocked with the game links already read, or nil to read
// them now.
func (s *API) needsKeyOfLocked(read []*app.GameLink) (scope, key, index, list string) {
	st, _ := s.a.Loadout()
	if st == nil {
		return
	}
	scope = s.needsScopeLocked()
	index = s.a.Store.IndexKey()
	list = app.ListFingerprint(st.List)
	cur, prev := s.a.LogPaths(st)
	// Links are outside SQLite. Probe them even on a hit; a removed junction target
	// must not hide behind a scan or database version.
	links, err := read, error(nil)
	if links == nil {
		links, err = s.a.InspectGameLinks()
	}
	b, _ := json.Marshal(links)
	lh := sha256.Sum256(b)
	dirs, _ := s.a.ManagedDirectories()
	db, _ := json.Marshal(dirs)
	dh := sha256.Sum256(db)
	// The save Can it run checks (12 §4.8): a chosen colony's newest save changes when a
	// save is written into its folder; the choice itself is in the store's change key.
	if cs := s.a.SaveChoiceStamp(st.Ix.Env.ID); cs != "" {
		dh = sha256.Sum256([]byte(string(dh[:]) + cs))
	}
	key = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%x|%v", scope, s.a.Store.ChangeKey(), list, s.a.AnalysisKey(st, ""), fileNeedsStamp(cur), fileNeedsStamp(prev), fileNeedsStamp(s.a.ProblemLog(st)), lh, err) + fmt.Sprintf("|%x", dh)
	return
}

// needsScopeLocked is the scope part of needsKeyLocked, which needs no look at the disk.
func (s *API) needsScopeLocked() string {
	st, _ := s.a.Loadout()
	if st == nil {
		return ""
	}
	return fmt.Sprintf("%s|%d|%s|%s", st.Ix.Env.ID, st.Plan.ID, st.GameMM(), view.Lang())
}

func (s *API) cachedNeedsLocked() (*needsSnapshot, string) {
	scope, key, _, _ := s.needsKeyLocked()
	return s.needsCache[scope], key
}
func (s *API) saveNeedsLocked(ps *app.ProblemSet, n *view.Needs, eager bool) *needsSnapshot {
	return s.saveNeedsOfLocked(ps, n, eager, false)
}

// saveFreshNeedsLocked is saveNeedsLocked for a ProblemSet that Problems has just read: its
// game links are the ones the key describes, so they are not read a third time (one look
// takes 0.1-0.25 s with a few hundred links, and the launch page saves at every start).
func (s *API) saveFreshNeedsLocked(ps *app.ProblemSet, n *view.Needs, eager bool) *needsSnapshot {
	return s.saveNeedsOfLocked(ps, n, eager, true)
}

func (s *API) saveNeedsOfLocked(ps *app.ProblemSet, n *view.Needs, eager, fresh bool) *needsSnapshot {
	if ps == nil {
		return nil
	}
	*n = *view.RebindNeedsLinks(s.a, n, ps.GameLinks)
	var read []*app.GameLink
	if fresh && ps.GameLinks != nil {
		read = ps.GameLinks
	}
	scope, key, index, list := s.needsKeyOfLocked(read)
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
	n.Updating = s.needsWorkLocked()
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
		s.workspace = &workspaceSnapshot{key: key, state: w.state, result: &old, boardAt: w.boardAt}
	}
	return c
}

// needsWorkLocked reports whether background work will replace the shown findings: a
// refresh of the list's problems, a content analysis, or one about to start by itself
// (analyzeTimer, 16 §8.9). Only then is `updating` true (16 §8.7): a cache-only read, a
// stale or a never-run analysis alone is not work.
func (s *API) needsWorkLocked() bool {
	return s.needsRefreshing || s.busy["analyze"] || s.analyzeTimer != nil
}

// problemsNowLocked is the summary of n with `updating` as of now.
func (s *API) problemsNowLocked(n *view.Needs) *view.ListProblems {
	p := view.ProblemsOf(n)
	p.Updating = s.needsWorkLocked()
	return p
}

// ListProblems supplies editor cards and per-mod counts without a second calculation.
func (s *API) ListProblems() *view.ListProblems {
	return view.ProblemsOf(s.Needs())
}
func (s *API) editProblemsLocked() *view.ListProblems {
	scope, _, index, list := s.needsKeyLocked()
	previous := s.needsCache[scope]
	if previous != nil && previous.index == index {
		if previous.list == list {
			return s.problemsNowLocked(s.needsLocked())
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
			return s.problemsNowLocked(n)
		}
	}
	n := s.needsLocked()
	if c := s.needsCache[scope]; c != nil {
		s.refreshNeedsLocked(c)
	}
	return s.problemsNowLocked(n)
}

// refreshNeedsLocked coalesces work on an independent session. Publication compares the
// input key again, so a slow log/analysis cannot overwrite a newer edit or environment.
func (s *API) refreshNeedsLocked(c *needsSnapshot) {
	if c == nil || s.needsClosed || s.needsRefreshing || s.tasks == nil {
		return
	}
	_, diskKey := s.cachedNeedsLocked()
	if c.eager && diskKey == c.key {
		return
	}
	expected := s.inventoryKey()
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
			s.saveFreshNeedsLocked(ps, n, true)
			s.autoAnalyzeLocked(n, true)
			event = s.problemsNowLocked(n)
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

// autoAnalyzeLocked keeps the shown findings current, the automatic part of 16 §8.9:
// wherever a list's findings are read (Can it run, the editor's problems, Launch) and its
// files' check is stale or never ran, a check follows by itself. Edits settle first: each
// call puts the start analyzeSettle later (analyzeSoonLocked), so a run of edits ends in
// one check of the final list. Entering a page (settle false) starts it at once. s.mu is
// held; n is what was just read (nil: analyzeSettled reads the state itself).
func (s *API) autoAnalyzeLocked(n *view.Needs, settle bool) bool {
	if s.needsClosed || n != nil && (n.Error != "" || !(n.Stale || n.Never)) {
		return false
	}
	d := time.Duration(0)
	if settle {
		d = s.analyzeSettle
		if d == 0 {
			d = analyzeSettle
		}
	}
	s.analyzeSoonLocked(d)
	return true
}

// analyzeSettle: how long after the last edit the automatic check starts.
const analyzeSettle = 2 * time.Second

// analyzeSoonLocked (re)arms the one pending automatic check d from now; s.mu is held.
func (s *API) analyzeSoonLocked(d time.Duration) {
	if s.analyzeTimer != nil {
		s.analyzeTimer.Stop()
	}
	s.analyzeTimer = time.AfterFunc(d, s.analyzeSettled) // not scheduled: an edit settle (16 §8.9), once per edit run
}

// analyzeSettled starts the automatic check if the open list still needs one. The newest
// state wins: a check already running is left to end (it writes its result only at the
// end) and one more follows it for the list as it is then (analyzeAfter). The same inputs
// (scope and app.AnalysisKey) start by themselves once a session: a check that failed or
// left them stale waits for check again, so nothing loops.
func (s *API) analyzeSettled() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.analyzeTimer = nil
	if s.needsClosed {
		return
	}
	if s.busy["analyze"] {
		s.analyzeAfter = true
		return
	}
	started := false
	defer func() {
		// not needed after all: the findings shown as updating are the current ones
		if !started && !s.needsWorkLocked() {
			if c, _ := s.cachedNeedsLocked(); c != nil {
				ev := s.problemsNowLocked(c.n)
				go s.send("list-problems", ev)
			}
		}
	}()
	st, err := s.a.Loadout()
	if err != nil || st == nil {
		return
	}
	if as, err := s.a.Analysis(st); err != nil || as.Run != nil && as.Fresh {
		return
	}
	key := s.needsScopeLocked() + "|" + s.a.AnalysisKey(st, "")
	if s.autoAnalyzed[key] || key == s.analyzedKey {
		return
	}
	if !s.startAnalyze() {
		return
	}
	started = true
	if s.autoAnalyzed == nil {
		s.autoAnalyzed = map[string]bool{}
	}
	s.autoAnalyzed[key] = true
}
