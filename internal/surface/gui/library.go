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
	return s.text(view.LibDo(s.a, kind, refs))
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

func openDir(dir string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	}
	return exec.Command("xdg-open", dir).Start()
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
