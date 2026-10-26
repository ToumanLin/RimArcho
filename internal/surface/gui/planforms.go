package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// PlanSources supplies the four starting points without changing the current list (06 §2.17).
type PlanSources struct {
	Copy     string          `json:"copy"`
	Count    int             `json:"count"`
	Versions []view.PlanFrom `json:"versions"`
	Saves    []view.PlanFrom `json:"saves"`
}

func (s *API) PlanSources(id int64) (*PlanSources, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.a.Plans()
	if err != nil {
		return nil, err
	}
	out := &PlanSources{Versions: []view.PlanFrom{}, Saves: []view.PlanFrom{}}
	found := false
	for _, p := range ps {
		if p.ID != id {
			continue
		}
		found = true
		out.Copy = view.Tf("a copy of %s", p.Name)
		out.Count = p.Entries
		vs, err := s.a.Store.Versions(id)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			label := fmt.Sprintf("v%d  %s %s  %s", v.N, view.ShortDate(v.Created), v.Created.Local().Format("15:04"), view.Tf("%d mods", len(loadout.Decode(v.Entries))))
			out.Versions = append(out.Versions, view.PlanFrom{From: fmt.Sprintf("v%d", v.N), Label: label})
		}
	}
	if !found {
		return nil, errors.New(view.T("that list is gone"))
	}
	e, err := s.a.Env()
	if err != nil {
		return nil, err
	}
	if e.UserDataRoot != "" {
		dir := filepath.Join(e.UserDataRoot, "Saves")
		files, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		for _, f := range files {
			if !f.IsDir() && strings.EqualFold(filepath.Ext(f.Name()), ".rws") {
				out.Saves = append(out.Saves, view.PlanFrom{From: filepath.Join(dir, f.Name()), Label: f.Name()})
			}
		}
	}
	return out, nil
}

// withPlan opens an explicit management target while the API lock is held; the current
// list (what launches) does not change. Failed edits reopen the previous list (06 §2.17).
func (s *API) withPlan(id int64, keep bool, run func() error) (err error) {
	previous, err := s.a.CurrentPlan()
	if err != nil {
		return err
	}
	if id != 0 && id != previous.ID {
		if _, err = view.OpenPlan(s.a, id); err != nil {
			return err
		}
		defer func() {
			if !keep || err != nil {
				_, restore := view.OpenPlan(s.a, previous.ID)
				if err == nil {
					err = restore
				}
			}
		}()
	}
	return run()
}

// CreatePlan copies the selected list, version or save and opens the resulting list (06 §2.17).
func (s *API) CreatePlan(source int64, name, from string) (done *PlanDone, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.withPlan(source, true, func() error {
		p, text, e := view.NewPlan(s.a, name, from)
		if e != nil {
			return e
		}
		done, e = s.planDone(p.ID, text, nil)
		return e
	})
	return
}

// ImportPreviewFor compares with the selected list and restores the editor's list (06 §2.17).
type PlanImportPreview struct {
	*view.ImportPreview
	Request uint64 `json:"request"`
}

func (s *API) ImportPreviewFor(id int64, kind, src string) (*PlanImportPreview, error) {
	s.mu.Lock()
	s.x.seq++
	seq := s.x.seq
	s.x.imp = nil
	r, err := s.a.ReadSession()
	var envID string
	if err == nil {
		var e error
		env, e := s.a.Env()
		if e != nil {
			r.Close()
			err = e
		} else {
			r.EnvName, envID = env.Name, env.ID
			r.SteamCollectionAPI = s.a.SteamCollectionAPI
		}
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	imp, err := view.ReadImport(r, kind, src)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if env, e := s.a.Env(); e != nil || env.ID != envID {
		return nil, errors.New("import environment changed")
	}
	if seq != s.x.seq {
		return nil, errors.New("import preview superseded")
	}
	preview, err := view.PreviewImportFor(s.a, imp, id)
	if err == nil {
		s.x.imp, s.x.follow = imp, imp.Collection != nil
		s.x.envID = envID
	}
	if err != nil {
		return nil, err
	}
	return &PlanImportPreview{ImportPreview: preview, Request: seq}, nil
}

// Create only the import the form actually previewed, even after another page read one.
func (s *API) ImportAsPlanPreview(name string, request uint64, follow bool) (*PlanDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request == 0 || request != s.x.seq || s.x.imp == nil {
		return nil, errors.New(view.T("the import preview changed; preview the content again"))
	}
	env, err := s.a.Env()
	if err != nil {
		return nil, err
	}
	if env.ID != s.x.envID {
		return nil, errors.New(view.T("the import preview changed; preview the content again"))
	}
	s.x.follow = follow
	return s.importAsPlan(name)
}

// ImportApplyTo edits the selected destination in one undoable step (06 §2.17).
func (s *API) ImportApplyTo(id int64, merge bool) (text string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.withPlan(id, true, func() error {
		if s.x.imp == nil {
			return errors.New(view.T("nothing to import"))
		}
		imp := s.x.imp
		var e error
		text, e = view.ApplyImport(s.a, imp, merge)
		if e != nil {
			return e
		}
		s.x.imp = nil
		if s.x.follow {
			if e := view.FollowImported(s.a, imp, id); e != nil {
				text += "  " + e.Error()
			}
		}
		s.board, s.needs = nil, nil
		return nil
	})
	return
}
