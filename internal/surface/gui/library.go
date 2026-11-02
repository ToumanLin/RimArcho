package gui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/perftrace"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Mod library > All (06 §2.9). The library is built once per database state; each filter
// click only filters it again.

// inventoryKey includes invocation options as well as database changes (opening a
// different list need not write to SQLite).
func (s *API) inventoryKey() string {
	return fmt.Sprintf("%s|%s|%d|%s|%s", s.a.Store.ChangeKey(), s.a.EnvName, s.a.OpenPlan, s.a.PlanName, s.a.GameVersion)
}

func (s *API) library() (*app.Library, error) {
	s.libGate.Lock()
	defer s.libGate.Unlock()
	s.libRead.mu.Lock()
	defer s.libRead.mu.Unlock()
	s.mu.Lock()
	key := s.inventoryKey()
	if s.lib != nil && s.libAt == key {
		lib := s.lib
		s.mu.Unlock()
		return lib, nil
	}
	r, err := s.inventorySession(&s.libRead)
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
	lib, err := r.Library()
	if err != nil {
		return nil, err
	}
	release()
	s.mu.Lock()
	if s.inventoryKey() == key {
		s.lib, s.libAt = lib, key
	}
	s.mu.Unlock()
	return lib, nil
}

// readInventorySession captures settings and a detached warm index; caller holds mu.
func (s *API) readInventorySession() (*app.App, error) {
	r, err := s.a.ReadSession()
	if err != nil {
		return nil, err
	}
	s.refreshInventorySession(r)
	r.WorkspaceSnapshot()
	return r, nil
}

func (s *API) refreshInventorySession(r *app.App) {
	r.ReadOptions(s.a)
	if e, err := s.a.Env(); err == nil {
		r.EnvName = e.Name
	}
	r.ReadIndex(s.a)
}

type inventoryReader struct {
	mu       sync.Mutex
	a        *app.App
	lib      *app.Library
	key, ref string
}

// withCopyLibrary serializes only detail readers. A full inventory build and editor
// verbs proceed independently. Keep just the last lineage, bounding retained memory.
func (s *API) withCopyLibrary(ref string, read func(*app.App, *app.Library) error) error {
	w := &s.copyRead
	w.mu.Lock()
	defer w.mu.Unlock()
	s.mu.Lock()
	key := s.inventoryKey()
	var err error
	if w.a == nil {
		w.a, err = s.readInventorySession()
	} else {
		s.refreshInventorySession(w.a)
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if w.lib == nil || w.key != key || (w.ref != ref && w.lib.Instance(ref) == nil) {
		lib, err := w.a.LibraryFor(ref)
		if err != nil {
			return err
		}
		w.lib, w.key, w.ref = lib, key, ref
	}
	return read(w.a, w.lib)
}

// Library is the page for a search, filters ("key:value", "-key:value" excludes) and a sort.
func (s *API) Library(search string, filters []string, sort string) (*view.LibPage, error) {
	defer s.free.soon()
	lib, err := s.library()
	if err != nil {
		return nil, err
	}
	return view.BuildLibrary(lib, search, filters, sort)
}

// LibraryPreview is the dry run of an action that changes files, for the confirmation.
func (s *API) LibraryPreview(kind string, refs []string) (*view.Preview, error) {
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	return view.LibPreview(a, kind, refs)
}

// LibraryDo runs an action on copies.
func (s *API) LibraryDo(kind string, refs []string) (string, error) {
	if kind == view.OpTextures {
		return s.TexturesRun("", 100, refs)
	}
	if kind == view.OpTexClean {
		return s.cleanTextures("", refs)
	}
	if view.TaskLane(kind) != "" && len(refs) > 0 {
		return s.submitTask(kind, refs)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.board, s.needs, s.lib = nil, nil, nil
	// the actions on folders and through Steam are background tasks (06 §2.16): queued
	// here, one item at a time, one failure does not stop the rest
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

// OpenSteamWorkshop opens a known copy or an uninstalled Workshop reference in Steam.
func (s *API) OpenSteamWorkshop(ref string) (string, error) {
	ws := ""
	if strings.HasPrefix(ref, "ws:") {
		ws = strings.TrimPrefix(ref, "ws:")
	} else {
		s.mu.Lock()
		ix, err := s.a.Index()
		s.mu.Unlock()
		if err != nil {
			return "", err
		}
		if mod := ix.ByID[ref]; mod != nil {
			ws = mod.WorkshopID()
		}
	}
	if !view.IsNumber(ws) {
		return "", errors.New(view.T("no Workshop ID: not a Workshop mod"))
	}
	if err := s.open(view.WorkshopSteamURL(ws)); err != nil {
		return "", err
	}
	return view.T("opened the Steam page"), nil
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

// ModCopies is the full detail's Mod library part (06 §4.2): the Lineage row of the mod with
// the instance id (or the copy ref), every copy with its state, place, size, version, note,
// plans and ops, and the op table they refer to.
type ModCopies struct {
	Row *view.LibRow `json:"row"`
	Ops []view.Op    `json:"opTable"`
}

func (s *API) ModCopies(id string) (*ModCopies, error) {
	var out *ModCopies
	err := s.withCopyLibrary(id, func(_ *app.App, lib *app.Library) error {
		p, err := view.BuildLibrary(lib, "", nil, "")
		if err != nil {
			return err
		}
		out = &ModCopies{Ops: p.Ops}
		for i := range p.Rows {
			for _, c := range p.Rows[i].Insts {
				if (id != "" && c.ID == id) || c.Ref == id {
					out.Row = &p.Rows[i]
					return nil
				}
			}
		}
		return nil
	})
	return out, err
}

// LibraryCopy is one copy's row (note, ops) by instance id or ref; nil when absent.
func (s *API) LibraryCopy(id string) (*view.LibInstRow, error) {
	defer perftrace.Span("LibraryCopy")()
	var out *view.LibInstRow
	err := s.withCopyLibrary(id, func(_ *app.App, lib *app.Library) error {
		out = view.LibCopy(lib, id)
		return nil
	})
	return out, err
}

// LibraryFields is a copy's field list, read on the same detached session as its lineage.
func (s *API) LibraryFields(ref string) ([]view.Field, error) {
	var out []view.Field
	err := s.withCopyLibrary(ref, func(r *app.App, lib *app.Library) error {
		i := lib.Instance(ref)
		if i == nil {
			return fmt.Errorf("no copy %s", ref)
		}
		var data *view.DetailData
		if i.Mod != nil {
			data = view.LoadDetail(r, lib.St.Ix, i.Mod, false, false)
		}
		out = view.LibFields(r, lib, i, data)
		return nil
	})
	return out, err
}

// LibrarySetNote sets a copy's own note ("" clears it).
func (s *API) LibrarySetNote(ref, text string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lib = nil
	return s.text(view.LibSetNote(s.a, ref, text))
}
