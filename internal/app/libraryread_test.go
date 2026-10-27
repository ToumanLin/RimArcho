package app_test

import (
	"reflect"
	"testing"
	"time"

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

func TestReadIndexOwnsLazyDeclarationsAndAdoption(t *testing.T) {
	t.Parallel()
	e := libFixture(t)
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.ReadIndex(e.App)
	rx, err := r.Index()
	if err != nil {
		t.Fatal(err)
	}
	id := ix.Mods[0].ID
	original, copy := ix.ByID[id], rx.ByID[id]
	if original == copy || ix.Env == rx.Env || ix.Sources[0] == rx.Sources[0] {
		t.Fatal("mutable index state shared")
	}
	original.DeclBlob, copy.DeclBlob = "", ""
	if copy.Decl().PackageID == "" || original.DeclBlob != "" {
		t.Fatal("reader did not own lazy declaration")
	}
	// Adopt and close the worker just as Workspace does. Future declaration reads
	// and localized lists must use the surviving session.
	if _, err := r.Loadout(); err != nil {
		t.Fatal(err)
	}
	e.App.AdoptReadState(r)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	copy.DeclBlob = ""
	if copy.Decl().PackageID == "" {
		t.Fatal("adopted mod still reads the closed worker DB")
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
