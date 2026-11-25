//go:build windows || cgo

package gui

import (
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestModPlayerData(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	mod, err := ix.ResolveOne("pkg:tester.b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Toggle(int(view.Inactive), []string{mod.ID}, nil); err != nil {
		t.Fatal(err)
	}
	assertNames := func(query, name, original string) {
		t.Helper()
		b := api.Board()
		if b.Error != "" {
			t.Fatal(b.Error)
		}
		col := b.Cols[view.Active]
		row := slices.IndexFunc(col, func(i view.Item) bool { return i.ID == mod.ID })
		if row < 0 || col[row].Name != name || col[row].OriginalName != original || col[row].Alias != (original != "") || b.Unsaved == 0 {
			t.Fatalf("editor row %d with %d unsaved edits: %+v", row, b.Unsaved, col)
		}
		filter, err := api.BarFilter(query, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(filter.Rows[view.Active]) != 1 || filter.Rows[view.Active][0] != row {
			t.Fatalf("editor search for %q: %+v", query, filter.Rows)
		}
		lib, err := api.Library(query, nil, "name")
		if err != nil {
			t.Fatal(err)
		}
		if len(lib.Rows) != 1 || lib.Rows[0].Name != name || lib.Rows[0].OriginalName != original {
			t.Fatalf("library search for %q: %+v", query, lib.Rows)
		}
		index, err := e.App.Index()
		if err != nil {
			t.Fatal(err)
		}
		if got := index.ByID[mod.ID].DisplayName(); got != name {
			t.Fatalf("DisplayName() = %q, want %q", got, name)
		}
	}
	assertNames("Beta", "Beta", "")
	if _, err := api.SetModAlias(mod.ID, "  Colony helper  "); err != nil {
		t.Fatal(err)
	}
	assertNames("Beta", "Colony helper", "Beta")
	assertNames("Colony helper", "Colony helper", "Beta")
	fields, err := api.LibraryFields(mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(fields, func(f view.Field) bool {
		return f.Kind == view.FieldAlias && f.Value == "Colony helper" && f.Extra == "Beta"
	}) {
		t.Fatalf("library fields missing alias and About.xml name: %+v", fields)
	}
	if _, err := api.SetModNote(mod.ID, "Lineage note"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.LibrarySetNote(mod.ID, "Copy note"); err != nil {
		t.Fatal(err)
	}
	d, err := api.Detail(mod.ID, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(d.Fields, func(f view.Field) bool {
		return f.Kind == view.FieldNote && f.Value == "Lineage note"
	}) {
		t.Fatalf("editor fields missing the Lineage note: %+v", d.Fields)
	}
	copy, err := api.LibraryCopy(mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if copy == nil || copy.Note != "Copy note" {
		t.Fatalf("library copy note: %+v", copy)
	}
	if _, err := api.SetModAlias(mod.ID, "  "); err != nil {
		t.Fatal(err)
	}
	assertNames("Beta", "Beta", "")
	if _, err := api.Undo(); err != nil {
		t.Fatal(err)
	}
	if b := api.Board(); b.Error != "" || b.Unsaved != 0 || slices.ContainsFunc(b.Cols[view.Active], func(i view.Item) bool { return i.ID == mod.ID }) {
		t.Fatalf("undo failed to remove Beta with %d unsaved edits: %s", b.Unsaved, b.Error)
	}
}
