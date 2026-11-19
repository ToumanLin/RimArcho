package gui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Saves › Play review (16 §2.11, 12 §3). The review attributes every save of the colonies
// asked for, reading first those without a current full read, so it runs in a goroutine
// on the Saves page's reader and answers with "review" events: a shell, partial pages,
// then the complete page. A newer request or leaving stops at the next save; the request
// then ends with a "stale" event, so the page lets go of it. The last page of each colony
// choice is kept, by Game Config, while the window is open and served again when no save
// changed since (the saves' paths, sizes and times, and the parser's version).

// reviewState is the review's newest request and its kept pages.
type reviewState struct {
	mu    sync.Mutex
	seq   uint64                 // the newest request; 0 after the page was left
	pages map[string]reviewEntry // by context (app.ReviewContextKey) and colony key ("*" all)
	// contexts are the contexts with kept pages, the one used last at the end.
	contexts []string
}

type reviewEntry struct {
	stamp string
	// saves is the listed saves' contexts as the page was made (ReviewSavesContextKey);
	// it depends on the current Game Config, so a mismatch makes the page unusable but
	// does not drop it: switching back can use it again.
	saves string
	page  *view.ReviewPage
}

// ReviewEvent carries page progress (Done/Total) and a final page or error; every request
// ends with one event that is not Partial, "stale" when a newer request or leaving the page
// stopped it.
type ReviewEvent struct {
	Request uint64           `json:"request"`
	Partial bool             `json:"partial"`
	Page    *view.ReviewPage `json:"page,omitempty"`
	Error   string           `json:"error,omitempty"`
}

// SaveReviewRead starts one colony ("" the latest), or all with "*"; the result
// is a "review" event with this request number. It returns at once.
func (s *API) SaveReviewRead(request uint64, colony string) bool {
	s.review.mu.Lock()
	if request > s.review.seq {
		s.review.seq = request
	}
	s.review.mu.Unlock()
	go func() {
		ev := s.runReview(request, colony)
		if ev == nil || s.reviewStale(request) {
			ev = &ReviewEvent{Request: request, Error: "stale"}
		}
		s.send("review", ev)
	}()
	return true
}

// leaveReview stops a running review; the kept pages stay.
func (s *API) leaveReview() {
	s.review.mu.Lock()
	s.review.seq = 0
	s.review.mu.Unlock()
}

func (s *API) reviewStale(request uint64) bool {
	s.review.mu.Lock()
	defer s.review.mu.Unlock()
	return request != s.review.seq
}

// runReview is one review on the Saves page's reader; nil when a newer request took over.
func (s *API) runReview(request uint64, colony string) *ReviewEvent {
	ev := &ReviewEvent{Request: request}
	w := &s.saves.read
	w.mu.Lock()
	defer w.mu.Unlock()
	defer s.retainReader(w)
	if s.reviewStale(request) {
		return nil
	}
	r, err := s.savesSession()
	if err != nil {
		ev.Error = err.Error()
		return ev
	}
	l, err := r.ListSaves()
	if err != nil {
		ev.Error = err.Error()
		return ev
	}
	cols := l.Colonies
	if colony != "*" {
		cols = nil
		for _, c := range l.Colonies {
			if c.Key == colony {
				cols = []*app.Colony{c}
			}
		}
		if cols == nil { // empty or gone: the first colony, as on the Saves page
			colony = ""
			if len(l.Colonies) > 0 {
				colony, cols = l.Colonies[0].Key, l.Colonies[:1]
			}
		}
	}
	if s.reviewStale(request) {
		return nil
	}
	total := 0
	for _, c := range cols {
		total += len(c.Saves)
	}
	// The shell is published before any full read, including on a kept-page hit.
	s.send("review", &ReviewEvent{Request: request, Partial: true, Page: view.ReviewPageOf(
		&app.SaveReview{Colonies: len(cols), Saves: total, Total: total}, l.Colonies, colony, nil)})
	input, err := r.ReviewInputKey()
	if err != nil {
		ev.Error = err.Error()
		return ev
	}
	// A save of no Game Config or list of its own is attributed in the current ones, so
	// each Game Config keeps its own pages.
	where, err := r.ReviewContextKey()
	if err != nil {
		ev.Error = err.Error()
		return ev
	}
	// every listed save's own context too: editing a list some save is attributed under
	// (not only the current one) makes kept pages stale
	saves, err := r.ReviewSavesContextKey(l.Colonies)
	if err != nil {
		ev.Error = err.Error()
		return ev
	}
	stamp := reviewStamp(l.Colonies) + input
	s.review.mu.Lock()
	// Only current pages survive. The keys are bounded by the Game Configs times this
	// listing's colonies + All.
	for key, entry := range s.review.pages {
		if entry.stamp != stamp {
			delete(s.review.pages, key)
		}
	}
	kept, ok := s.review.pages[where+"\x00"+colony]
	s.review.mu.Unlock()
	if ok && kept.stamp == stamp && kept.saves == saves {
		ev.Page = kept.page
		return ev
	}
	tags := reviewTags(r)
	res, err := r.SaveReview(cols, app.ReviewOpts{Read: true,
		Stop: func() bool { return s.reviewStale(request) },
		Partial: func(part *app.SaveReview) {
			if !s.reviewStale(request) {
				s.send("review", &ReviewEvent{Request: request, Partial: true, Page: view.ReviewPageOf(part, l.Colonies, colony, tags)})
			}
		}})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	if err != nil {
		ev.Error = err.Error()
		return ev
	}
	if s.reviewStale(request) {
		return nil
	}
	// Reading a save can move it to another colony (grouping by world, 12 §4.1): the page
	// names the colonies as they are now, and is kept under their saves as they are now.
	if l2, err := r.ListSaves(); err == nil {
		if colony != "*" && len(cols) > 0 {
			colony = ""
			for _, c := range l2.Colonies {
				for _, sv := range c.Saves {
					if sv.Path == cols[0].Saves[0].Path {
						colony, cols = c.Key, []*app.Colony{c}
					}
				}
			}
		}
		l, stamp = l2, reviewStamp(l2.Colonies)+input
		if k, err := r.ReviewSavesContextKey(l2.Colonies); err == nil {
			saves = k
		}
	}
	ev.Page = view.ReviewPageOf(res, l.Colonies, colony, tags)
	s.review.mu.Lock()
	if s.review.pages == nil {
		s.review.pages = map[string]reviewEntry{}
	}
	if request == s.review.seq {
		s.review.keep(where, colony, reviewEntry{stamp: stamp, saves: saves, page: ev.Page})
	}
	s.review.mu.Unlock()
	return ev
}

// reviewContexts is how many contexts (Game Config, version, list) keep pages: switching
// between two Game Configs serves both, and editing a list does not pile up pages.
const reviewContexts = 2

// keep stores a page of colony in context where, dropping the pages of the context used
// least recently beyond reviewContexts. The caller holds mu.
func (r *reviewState) keep(where, colony string, entry reviewEntry) {
	r.contexts = append(slices.DeleteFunc(r.contexts, func(c string) bool { return c == where }), where)
	if len(r.contexts) > reviewContexts {
		gone := r.contexts[0]
		r.contexts = r.contexts[1:]
		for key := range r.pages {
			if strings.HasPrefix(key, gone+"\x00") {
				delete(r.pages, key)
			}
		}
	}
	r.pages[where+"\x00"+colony] = entry
}

// reviewTags are the player's tags by library copy; none when the library cannot be read.
func reviewTags(r *app.App) map[string][]string {
	out := map[string][]string{}
	lib, err := r.Library()
	if err != nil {
		return out
	}
	for _, l := range lib.Lineages {
		if len(l.Tags) == 0 {
			continue
		}
		for _, i := range l.Instances {
			if i.Mod != nil {
				out[i.Mod.ID] = l.Tags
			}
		}
	}
	return out
}

// reviewStamp changes when a save of cols is added, removed or written, or the parser
// changes.
func reviewStamp(cols []*app.Colony) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d\n", savefile.ParserVersion)
	for _, c := range cols {
		fmt.Fprintf(h, "%s\n", c.Key)
		for _, sv := range c.Saves {
			fmt.Fprintf(h, "%s\n", factsKey(sv))
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// SaveReviewMark records the player's functional mark on the Mod of a library copy
// (12 §3.3 rule 2): app.MarkFunctional, app.MarkNotFunctional or "" to clear it. Kept
// pages are dropped, so the page's next request assembles them with the mark; no save is
// read or attributed again. It returns the status line.
func (s *API) SaveReviewMark(library, mark string) (string, error) {
	s.mu.Lock()
	m, err := s.a.SetFunctional("#"+library, mark)
	s.lib = nil
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	s.review.mu.Lock()
	s.review.pages = map[string]reviewEntry{}
	s.review.contexts = nil
	s.review.mu.Unlock()
	switch mark {
	case app.MarkFunctional:
		return view.Tf("%s marked functional", m.Name), nil
	case app.MarkNotFunctional:
		return view.Tf("%s marked not functional", m.Name), nil
	}
	return view.Tf("%s: mark removed", m.Name), nil
}
