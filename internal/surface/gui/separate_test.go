package gui

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The detail's action separates exactly the shown instance, inherits player data, keeps
// plan/file identity and refreshes cached details. A later scan must not rejoin it.
func TestSeparateCopyFromDetail(t *testing.T) {
	t.Parallel()
	for _, entry := range []string{"library", "editor"} {
		t.Run(entry, func(t *testing.T) {
			e, api, _ := fixture(t)
			copyPath := filepath.Join(e.Game, "Mods", "Alpha Copy")
			e.CopyTree(filepath.Join(e.Workshop, "1001"), copyPath)
			e.MustRun("scan")
			ix, err := e.App.Index()
			if err != nil {
				t.Fatal(err)
			}
			var cp, upstream *store.Mod
			for _, m := range ix.Mods {
				if m.PackageID == "tester.a" {
					if m.Kind == source.Local {
						cp = m
					} else {
						upstream = m
					}
				}
			}
			if cp == nil || upstream == nil || cp.LineageID != upstream.LineageID {
				t.Fatal("missing grouped copies")
			}
			oldLineage := cp.LineageID
			if err := e.App.Store.SetAlias(ix.Env.ID, oldLineage, "Alpha Alias"); err != nil {
				t.Fatal(err)
			}
			if err := e.App.Store.SetNote(ix.Env.ID, oldLineage, "shared note"); err != nil {
				t.Fatal(err)
			}
			if _, err := e.App.Store.AddTag(ix.Env.ID, oldLineage, "my tag"); err != nil {
				t.Fatal(err)
			}
			if err := e.App.Store.SetCopyNote(cp.ID, "copy note", e.App.Now()); err != nil {
				t.Fatal(err)
			}
			api.Board()
			before, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			aboutPath := filepath.Join(copyPath, "About", "About.xml")
			about, err := os.ReadFile(aboutPath)
			if err != nil {
				t.Fatal(err)
			}
			d, err := api.Detail(cp.ID, false, false, false)
			if err != nil || !d.CanSeparate {
				t.Fatalf("detail before split: %+v %v", d, err)
			}
			copies, err := api.ModCopies(cp.ID)
			if err != nil || copies.Row == nil || len(copies.Row.Insts) != 2 {
				t.Fatalf("copies before: %+v %v", copies, err)
			}
			for _, i := range copies.Row.Insts {
				if !slices.Contains(i.Ops, view.OpSeparate) {
					t.Fatalf("missing detail action: %+v", i)
				}
			}
			op := view.OpOf(view.OpSeparate)
			if !op.Detail || !op.One {
				t.Fatalf("not a single-copy detail button: %+v", op)
			}
			// A stale/multi-selection request must leave both instances grouped.
			if _, err := api.LibraryDo(view.OpSeparate, []string{ix.Ref(cp.ID), ix.Ref(upstream.ID)}); err == nil {
				t.Fatal("accepted multiple copies")
			}
			if entry == "library" {
				_, err = api.LibraryDo(view.OpSeparate, []string{ix.Ref(cp.ID)})
			} else {
				_, err = api.RunOp(view.OpSeparate, []string{cp.ID})
			}
			if err != nil {
				t.Fatal(err)
			}
			after, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			split := after.Ix.ByID[cp.ID]
			if split == nil || split.LineageID == oldLineage || split.LineageBasis != identity.BasisManual {
				t.Fatalf("not separated: %+v", split)
			}
			if after.Ix.ByID[upstream.ID].LineageID != oldLineage {
				t.Fatal("moved the upstream")
			}
			for _, m := range []*store.Mod{split, after.Ix.ByID[upstream.ID]} {
				if m.Alias != "Alpha Alias" || m.Note != "shared note" || !slices.Equal(m.Tags, []string{"my tag"}) {
					t.Fatalf("lost player data: %+v", m)
				}
			}
			if split.RealPath != cp.RealPath || split.SnapID != cp.SnapID || !reflect.DeepEqual(before.List, after.List) {
				t.Fatal("changed files, snapshot or plan")
			}
			if got, err := os.ReadFile(aboutPath); err != nil || string(got) != string(about) {
				t.Fatal("changed About.xml")
			}
			copies, err = api.ModCopies(cp.ID)
			if err != nil || copies.Row == nil || !copies.Row.Single || len(copies.Row.Insts) != 1 || copies.Row.Insts[0].ID != cp.ID || copies.Row.Insts[0].Note != "copy note" {
				t.Fatalf("stale copies or lost note: %+v %v", copies, err)
			}
			if slices.Contains(copies.Row.Insts[0].Ops, view.OpSeparate) {
				t.Fatal("still offers split on singleton")
			}
			d, err = api.Detail(cp.ID, false, false, false)
			if err != nil || d.CanSeparate {
				t.Fatalf("stale detail: %+v %v", d, err)
			}
			if _, err := api.RunOp(view.OpSeparate, []string{cp.ID}); err == nil {
				t.Fatal("repeated split succeeded")
			}
			if err := e.App.Store.SetNote(ix.Env.ID, split.LineageID, "independent note"); err != nil {
				t.Fatal(err)
			}
			e.MustRun("scan")
			ix, err = e.App.Index()
			if err != nil {
				t.Fatal(err)
			}
			if ix.ByID[cp.ID].LineageID != split.LineageID || ix.ByID[cp.ID].Note != "independent note" || ix.ByID[upstream.ID].Note != "shared note" {
				t.Fatal("scan undid split or data edits affected original")
			}
		})
	}
}
