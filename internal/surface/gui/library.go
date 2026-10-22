package gui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Mod 库存 › 全部 (06 §2.9). The library is built once per database state; each filter
// click only filters it again.

func (s *API) library() (*app.Library, error) {
	key := s.a.Store.ChangeKey()
	if s.lib != nil && s.libAt == key {
		return s.lib, nil
	}
	lib, err := s.a.Library()
	if err != nil {
		return nil, err
	}
	s.lib, s.libAt = lib, s.a.Store.ChangeKey()
	return lib, nil
}

// Library is the page for a search, filters ("key:value", "-key:value" excludes) and a sort.
func (s *API) Library(search string, filters []string, sort string) (*view.LibPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.free.soon()
	lib, err := s.library()
	if err != nil {
		return nil, err
	}
	return view.BuildLibrary(lib, search, filters, sort)
}

// LibraryPreview is the dry run of an action that changes files, for the confirmation.
func (s *API) LibraryPreview(kind string, refs []string) (*view.Preview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.LibPreview(s.a, kind, refs)
}

// LibraryDo runs an action on copies.
func (s *API) LibraryDo(kind string, refs []string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.board, s.needs, s.lib = nil, nil, nil
	t, err := view.LibDo(s.a, kind, refs)
	// Steam out of reach (not running, no DLL): the Workshop pages instead, said so (06 §2.9)
	var pages *view.OpenPages
	if errors.As(err, &pages) {
		for _, u := range pages.URLs {
			if err := s.open(u); err != nil {
				return "", err
			}
		}
		return pages.Msg, nil
	}
	return s.text(t, err)
}

// OpenFolder shows an instance's folder in the file manager. Only folders of known
// instances: the page never passes a path.
func (s *API) OpenFolder(id string) error {
	s.mu.Lock()
	ix, err := s.a.Index()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	m := ix.ByID[id]
	if m == nil {
		return fmt.Errorf("no mod %s", id)
	}
	dir := ix.ModDir(m)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return errors.New(view.T("the folder is not there"))
	}
	if s.openDir != nil {
		return s.openDir(dir)
	}
	return openDir(dir)
}

// OpenWorkshop opens an instance's Workshop page (ctrl+w, 06 §3.5); without a Workshop ID
// it says so.
func (s *API) OpenWorkshop(id string) (string, error) {
	s.mu.Lock()
	ix, err := s.a.Index()
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	m := ix.ByID[id]
	if m == nil || !view.IsNumber(m.WorkshopID()) {
		return "", errors.New(view.T("no Workshop ID: not a Workshop mod"))
	}
	if err := s.open(view.WorkshopURL(m.WorkshopID())); err != nil {
		return "", err
	}
	return view.T("opened the Workshop page"), nil
}

// A variable so tests can never start the real program (noopen_test.go).
var openDir = func(dir string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	}
	return exec.Command("xdg-open", dir).Start()
}

// ModCopies is the full detail's Mod 库存 part (06 §4.2): the Lineage row of the mod with
// the instance id (or the copy ref), every copy with its state, place, size, version, note,
// plans and ops, and the op table they refer to.
type ModCopies struct {
	Row *view.LibRow `json:"row"`
	Ops []view.Op    `json:"opTable"`
}

func (s *API) ModCopies(id string) (*ModCopies, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lib, err := s.library()
	if err != nil {
		return nil, err
	}
	p, err := view.BuildLibrary(lib, "", nil, "")
	if err != nil {
		return nil, err
	}
	for i := range p.Rows {
		for _, c := range p.Rows[i].Insts {
			if (id != "" && c.ID == id) || c.Ref == id {
				return &ModCopies{Row: &p.Rows[i], Ops: p.Ops}, nil
			}
		}
	}
	return &ModCopies{Ops: p.Ops}, nil
}

// LibraryFields is a copy's field list (06 §2.9): one list, the editor's fields with the
// library's (state, used in, size, versions, update date, last played) among them.
func (s *API) LibraryFields(ref string) ([]view.Field, error) {
	s.mu.Lock()
	lib, err := s.library()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	i := lib.Instance(ref)
	s.mu.Unlock()
	if i == nil {
		return nil, fmt.Errorf("no copy %s", ref)
	}
	var data *view.DetailData
	if i.Mod != nil {
		data = view.LoadDetail(s.a, lib.St.Ix, i.Mod, false, false) // the cache only: no network here
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.LibFields(s.a, lib, i, data), nil
}

// LibrarySetNote sets a copy's own note ("" clears it).
func (s *API) LibrarySetNote(ref, text string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lib = nil
	return s.text(view.LibSetNote(s.a, ref, text))
}
