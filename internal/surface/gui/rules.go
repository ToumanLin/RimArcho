package gui

import (
	"errors"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The plan editor's Load-order rules (06 §2.5, 04 §6.1): the player's load-order rules. They are
// global; a change reaches the sort and the problems through the database's change key.

// RuleSpec is a rule as the editor's form sends it: A and B are references (L#…, #…).
type RuleSpec struct {
	Kind   string `json:"kind"`
	A      string `json:"a"`
	B      string `json:"b"`
	Target string `json:"target"`
	Note   string `json:"note"`
}

// Rules lists the rules for the editor's fold.
func (s *API) Rules() (*view.Rules, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.BuildRules(s.a)
}

// RuleCandidates are the mods the form offers for the second mod while typing.
func (s *API) RuleCandidates(text, not string) ([]view.RuleMod, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.RuleCandidates(s.a, text, not)
}

// SaveRule adds a rule (id 0) or changes one; r:<n> stays.
func (s *API) SaveRule(id int64, r RuleSpec) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, err := view.SaveRule(s.a, id, app.RuleSpec{Kind: r.Kind, A: r.A, B: r.B, Target: r.Target, Note: r.Note})
	if err != nil {
		return "", errors.New(view.Humanize(s.boardState(), err.Error()))
	}
	s.board = nil
	return text, nil
}

// RemoveRule deletes a rule at once; RestoreRule (the status line's undo) puts it back.
func (s *API) RemoveRule(id int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, r, err := view.RemoveRule(s.a, id)
	if err != nil {
		return "", err
	}
	s.removedRule, s.board = r, nil
	return text, nil
}

// RestoreRule puts back the rule RemoveRule removed last.
func (s *API) RestoreRule(id int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.removedRule
	if r == nil || r.ID != id {
		return "", errors.New(view.T("nothing to undo"))
	}
	s.removedRule, s.board = nil, nil
	return view.RestoreRule(s.a, r)
}
