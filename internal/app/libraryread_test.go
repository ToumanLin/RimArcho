package app_test

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestLibraryForMatchesFullLineage(t *testing.T) {
	t.Parallel()
	e := libFixture(t)
	for _, m := range mustLib(t, e).St.Ix.Mods {
		if err := e.App.Store.SetCopyNote(m.ID, "copy note: "+m.ID, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	full := mustLib(t, e)
	page, err := view.BuildLibrary(full, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range page.Rows {
		for _, copy := range row.Insts {
			for _, ref := range []string{copy.ID, copy.Ref} {
				if ref == "" {
					continue
				}
				scoped, err := e.App.LibraryFor(ref)
				if err != nil {
					t.Fatal(err)
				}
				got, err := view.BuildLibrary(scoped, "", nil, "")
				if err != nil || len(got.Rows) != 1 || !reflect.DeepEqual(got.Rows[0], row) {
					t.Fatalf("lineage for %q differs: %+v, %v; want %+v", ref, got, err, row)
				}
			}
		}
	}
	for _, ref := range []string{"", "unknown"} {
		lib, err := e.App.LibraryFor(ref)
		if err != nil || len(lib.Lineages) != 0 {
			t.Fatalf("unknown copy %q: %+v, %v", ref, lib, err)
		}
	}
}

// Read sessions share an index's Mods (07 §8.3); their lazy declarations keep loading
// after the session the index came from closes, and after adoption.
func TestReadIndexSharesModsBeyondItsSource(t *testing.T) {
	t.Parallel()
	e := libFixture(t)
	w, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	wx, err := w.Index()
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.ReadIndex(w)
	rx, err := r.Index()
	if err != nil {
		t.Fatal(err)
	}
	id := wx.Mods[0].ID
	m := rx.ByID[id]
	if m != wx.ByID[id] || wx.Env == rx.Env {
		t.Fatal("the reader should share the Mods and own its environment")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	// Sessions read shared declarations concurrently.
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, x := range rx.Mods {
				x.ForgetDecl()
				if x.SnapID != 0 && x.Decl().PackageID == "" {
					t.Error("a shared Mod read its declaration from the closed source session")
					return
				}
			}
		}()
	}
	wg.Wait()
	// Adopt and close the worker just as Workspace does.
	if _, err := r.Loadout(); err != nil {
		t.Fatal(err)
	}
	e.App.AdoptReadState(r)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	m.ForgetDecl()
	if m.Decl().PackageID == "" {
		t.Fatal("adopted mod still reads the closed worker DB")
	}
	if err := e.App.Store.LoadDecls(rx.Mods); err != nil || !m.DeclLoaded() {
		t.Fatalf("batch load: %v", err)
	}
}

// Without the declaration connection (an App not from Open) a reader loads its own index.
func TestReadIndexWithoutDeclConnection(t *testing.T) {
	t.Parallel()
	e := libFixture(t)
	src := &app.App{Store: e.App.Store}
	src.ReadOptions(e.App)
	if _, err := src.Index(); err != nil {
		t.Fatal(err)
	}
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.ReadIndex(src)
	if r.IndexCached() {
		t.Fatal("an App without the declaration connection took a shared index")
	}
}

func TestReadIndexDoesNotStampOldSnapshotAsCurrent(t *testing.T) {
	t.Parallel()
	e := libFixture(t)
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	release, err := r.Store.ReadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ix, err := r.Index()
	if err != nil {
		t.Fatal(err)
	}
	m := ix.Mods[0]
	if err := e.App.Store.SetAlias("", m.LineageID, "Newer committed name"); err != nil {
		t.Fatal(err)
	}
	next, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	// r still sees the pinned, older generation; next sees the new commit.
	next.ReadIndex(r)
	got, err := next.Index()
	if err != nil || got.ByID[m.ID].DisplayName() != "Newer committed name" {
		t.Fatalf("copied snapshot hid a newer commit: %v", err)
	}
}
