package gui

import (
	"errors"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/perftrace"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The plan editor's filter bar, the ops both pages share and the help per page (06 §2.5,
// §2.9, §3.5, §8.5).

// BarFilter is the editor's filter bar over 未启用 and 已启用: the rows that match, and the
// groups while the bar is open.
func (s *API) BarFilter(search string, filters []string, groups bool) (*view.BarFilter, error) {
	defer perftrace.Span("BarFilter")()
	// Filtering follows Workspace and uses exactly its row order. Keep that
	// reader's immutable rows; revalidate the index and list on every request.
	w := &s.workspaceReader
	w.mu.Lock()
	defer w.mu.Unlock()
	s.mu.Lock()
	r, err := s.inventorySession(w)
	s.mu.Unlock()
	if err == nil {
		err = s.refreshPageIndex(r)
	}
	if err != nil {
		return nil, err
	}
	release, err := pinPageRead(r)
	if err != nil {
		return nil, err
	}
	defer release()
	reader := &API{a: r, busy: map[string]bool{}, board: w.board, boardAt: w.boardAt}
	b := reader.boardLocked()
	w.board, w.boardAt = reader.board, reader.boardAt
	if b.Error != "" {
		return nil, errors.New(b.Error)
	}
	if b.Board == nil || b.St == nil {
		return &view.BarFilter{}, nil
	}
	f := view.MatchFacts{Status: b.Status}
	if view.BarNeedsContent(search, filters, groups) {
		c, err := reader.content.Get(r, b.St)
		if err != nil {
			return nil, err
		}
		f.Content = c
	}
	return view.FilterBoard(b.Board, search, filters, f, groups, b.St.GameMM()), nil
}

// MoveShown is J / K while the bar filters 已启用: past the next shown row (view.StepAt).
// shown are the load positions of the rows shown, in order.
func (s *API) MoveShown(ids []string, d int, shown []int) (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return nil, errors.New("no list")
	}
	ps := view.Positions(st, ids)
	if len(ps) == 0 {
		return nil, errors.New(view.T("only enabled mods can be moved"))
	}
	at := view.StepAt(shown, ps, d)
	if at == 0 {
		return &view.Result{Text: view.T("already at the edge of the rows shown")}, nil
	}
	return s.after(view.Place(s.a, view.Active, s.object(ids, nil), at))
}

// RowOps are the op codes of an editor row (its copy's, with the column's plan ops).
func (s *API) RowOps(col int, id string) []string {
	out := view.RowOps(nil, id, view.Column(col))
	_ = s.withCopyLibrary(id, func(_ *app.App, lib *app.Library) error {
		out = view.RowOps(lib, id, view.Column(col))
		return nil
	})
	return out
}

// RunOp runs an op of the version and disk groups (and 从所有方案里拿掉) on editor rows; the
// plan ops have their own verbs.
func (s *API) RunOp(code string, ids []string) (string, error) {
	s.mu.Lock()
	st := s.boardState()
	s.mu.Unlock()
	if st == nil {
		return "", errors.New("no list")
	}
	var refs []string
	for _, id := range ids {
		refs = append(refs, st.Ix.Ref(id))
	}
	return s.LibraryDo(code, refs)
}

// OpPreview is the dry run of an editor row's op that changes files.
func (s *API) OpPreview(code string, ids []string) (*view.Preview, error) {
	s.mu.Lock()
	st := s.boardState()
	s.mu.Unlock()
	if st == nil {
		return nil, errors.New("no list")
	}
	var refs []string
	for _, id := range ids {
		refs = append(refs, st.Ix.Ref(id))
	}
	return s.LibraryPreview(code, refs)
}

// Keys is the help of a page (`?`, 06 §3.5).
func (s *API) Keys(page string) []view.KeyRow { return view.KeyHelp(page, false) }

// ModNote is a mod's own note (n).
func (s *API) ModNote(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil || st.Ix.ByID[id] == nil {
		return ""
	}
	return st.Ix.ByID[id].Note
}

// SetModNote sets a mod's own note ("" clears it).
func (s *API) SetModNote(id, text string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil || st.Ix.ByID[id] == nil {
		return "", errors.New("no mod " + id)
	}
	s.board, s.lib = nil, nil
	return s.text(view.SetModNote(s.a, st.Ix.Ref(id), text))
}

// CompleteDependencies enables installed prerequisites and sorts the open plan.
// Subscription remains a separate Subscribe call after choosing Steam or SteamCMD.
func (s *API) CompleteDependencies() (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needs, s.board = nil, nil
	return s.after(view.CompleteDependencies(s.a))
}

// ReplaceRecommended replaces the named enabled originals with their installed
// recommendations, validated again at application time, as one undoable list edit.
func (s *API) ReplaceRecommended(refs []string) (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needs, s.board = nil, nil
	return s.after(view.ReplaceRecommended(s.a, refs))
}

// UseInstalled binds missing package IDs to usable installed copies in one undo step.
func (s *API) UseInstalled(packageIDs []string) (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needs, s.board = nil, nil
	return s.after(view.UseInstalled(s.a, packageIDs))
}
