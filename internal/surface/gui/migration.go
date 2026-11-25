//go:build windows || cgo

package gui

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Moving in from another manager, part by part, as a background task (16 §2.18, §2.16).

// ImportSurvey says what the managers found on this computer hold, part by part, and what
// came in before. extra are files the player chose with "another folder or file…". It
// reads in its own session (dry runs only), so the window stays free.
func (s *API) ImportSurvey(extra []app.ManagerImport) ([]app.MigrationManager, error) {
	found := app.FindManagerImports(s.gameModsDirs())
	seen := map[string]bool{}
	for _, f := range found {
		seen[f.Kind+"|"+strings.ToLower(filepath.Clean(f.Path))] = true
	}
	for _, f := range extra {
		if f.Kind == "" || f.Path == "" || seen[f.Kind+"|"+strings.ToLower(filepath.Clean(f.Path))] {
			continue
		}
		if f.Manager == "" {
			f.Manager = map[string]string{"rimcrow": "RimCrow", "rimsort-aux": "RimSort", "rimsort-rules": "RimSort", "rimstack": "RimStack", "rimstack-rules": "RimStack"}[f.Kind]
		}
		found = append(found, f)
	}
	r, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	s.mu.Lock()
	r.ReadIndex(s.a)
	s.mu.Unlock()
	out := r.SurveyMigration(found, view.ManagerTags())
	for i := range out {
		for j := range out[i].Groups {
			g := &out[i].Groups[j]
			if g.Error != "" {
				g.Error = view.Humanize(nil, g.Error)
			}
			for k := range g.Parts {
				if p := &g.Parts[k]; p.State == app.PartBlocked && p.Why != "" {
					p.Why = view.ProfileStatus(p.Why)
				}
			}
		}
	}
	return out, nil
}

// MigrationPickIn is one part the page picked: its ID, the mode for a list imported
// before (copy, update) and the name its line shows.
type MigrationPickIn struct {
	ID   string `json:"id"`
	Mode string `json:"mode,omitempty"`
	Name string `json:"name"`
}

// StartMigration brings the picked parts over as one background task, an item per part;
// it returns the task's ID. obj names the managers ("RimCrow and RimSort").
func (s *API) StartMigration(picks []MigrationPickIn, obj string) (string, error) {
	if len(picks) == 0 {
		return "", errors.New(view.T("nothing selected"))
	}
	parsed := make([]app.MigrationPick, len(picks))
	items := make([]app.TaskItem, len(picks))
	for i, p := range picks {
		mp, err := app.ParseMigrationID(p.ID)
		if err != nil {
			return "", err
		}
		mp.Mode = p.Mode
		parsed[i] = mp
		items[i] = app.TaskItem{Ref: p.ID, Name: p.Name, State: app.ItemWait}
	}
	id := s.tasks.Begin(view.TaskMigrate, obj, func() error { return nil })
	s.tasks.Update(id, func(t *app.Task) { t.Items, t.Total = items, len(items) })
	go s.runMigration(id, parsed)
	return id, nil
}

func (s *API) runMigration(id string, picks []app.MigrationPick) {
	ok, failed := 0, 0
	for i, p := range picks {
		if s.tasks.StopAsked(id) {
			s.tasks.Update(id, func(t *app.Task) {
				for j := i; j < len(t.Items); j++ {
					t.Items[j].State = app.ItemSkip
				}
			})
			s.tasks.Finish(id, app.TaskStopped, view.Tf("%d of %d parts brought over; stopped", ok, len(picks)))
			return
		}
		s.tasks.Update(id, func(t *app.Task) { t.Items[i].State, t.Phase = app.ItemRun, t.Items[i].Name })
		note, err := s.migrateOne(p)
		s.tasks.Update(id, func(t *app.Task) {
			if err != nil {
				t.Items[i].State, t.Items[i].Why = app.ItemFail, view.Humanize(nil, err.Error())
			} else {
				t.Items[i].State, t.Items[i].Note = app.ItemOK, note
			}
			t.Done = i + 1
		})
		if err != nil {
			failed++
		} else {
			ok++
		}
	}
	state := app.TaskDone
	switch {
	case failed > 0 && ok == 0:
		state = app.TaskFailed
	case failed > 0:
		state = app.TaskPartial
	}
	s.tasks.Finish(id, state, view.Tf("%d of %d parts brought over", ok, len(picks)))
}

// migrateOne brings one part over while it holds the window's lock; the window answers
// between parts.
func (s *API) migrateOne(p app.MigrationPick) (string, error) {
	release, err := s.lockEdit()
	if err != nil {
		return "", err
	}
	defer release()
	defer func() { s.board, s.needs = nil, nil }()
	if p.Source == "list" && p.Mode == "update" {
		plan := s.a.ImportedPlan(p.Path)
		if plan == 0 {
			return "", errors.New(view.T("the list it became is gone"))
		}
		imp, err := view.ReadImport(s.a, view.ImportFile, p.Path)
		if err != nil {
			return "", err
		}
		var text string
		err = s.withPlan(plan, false, func() error {
			var e error
			text, e = view.ApplyImport(s.a, imp, false)
			return e
		})
		if err != nil {
			return "", err
		}
		return text, s.a.RecordImport(p.Path, plan)
	}
	out, err := s.a.MigratePart(p, view.ManagerTags())
	if err != nil {
		return "", err
	}
	return view.MigrationText(p, out), nil
}

// TaskByID is one task's record, for a page that shows its own task in place.
func (s *API) TaskByID(id string) *app.Task {
	for _, t := range s.tasks.List() {
		if t.ID == id {
			return &t
		}
	}
	return nil
}

// StartPackImport adds a pack from an .archopack file as a background task (10 §4) and
// returns its ID; the task's result is PackImport's text.
// With update it applies a newer revision of a pack the player has (PackUpdateOffer).
func (s *API) StartPackImport(path string, update bool) (string, error) {
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	if !app.IsPackZip(path) {
		return "", errors.New(view.Tf("%s is not a pack (.archopack)", path))
	}
	id := s.tasks.Begin(view.TaskPackImport, filepath.Base(path), nil)
	go func() {
		text, err := s.packImport(path, update)
		s.endSelf(id, err, text)
	}()
	return id, nil
}

// ManagerPartView is what one part of another manager's data holds, read without bringing it
// over: the Import page's "view" (16 §2.18). A list or a Game Config gives its list file (the
// page compares it); Mod data and rules give their values one by one.
func (s *API) ManagerPartView(id string) (*app.PartView, error) {
	p, err := app.ParseMigrationID(id)
	if err != nil {
		return nil, err
	}
	r, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	s.mu.Lock()
	r.ReadIndex(s.a)
	s.mu.Unlock()
	v, err := r.MigrationPartView(p, view.ManagerTags())
	if err != nil {
		return nil, errors.New(view.Humanize(nil, err.Error()))
	}
	return v, nil
}
