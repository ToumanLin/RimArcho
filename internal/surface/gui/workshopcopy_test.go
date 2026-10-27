package gui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestWorkshopCopyUpdateEntryPoints(t *testing.T) {
	t.Parallel()
	for _, entry := range []string{"library", "editor"} {
		t.Run(entry, func(t *testing.T) {
			e, api, _ := fixture(t)
			e.App.GameProbe = func(string) *app.GameRun { return nil }
			e.App.TrashFunc = os.RemoveAll
			target := filepath.Join(e.Game, "Mods", "Alpha Copy")
			e.CopyTree(filepath.Join(e.Workshop, "1001"), target)
			e.MustRun("scan")
			ix, err := e.App.Index()
			if err != nil {
				t.Fatal(err)
			}
			cp := ix.ModAt(target)
			if cp == nil || cp.Kind != source.Local {
				t.Fatal("missing copy")
			}
			e.WriteFile(filepath.Join(target, "local.txt"), "discard me")
			api.Board()
			copy, err := api.LibraryCopy(cp.ID)
			if err != nil || !slices.Contains(copy.Ops, view.OpPinUpdate) {
				t.Fatalf("copy ops: %+v %v", copy, err)
			}
			var preview *view.Preview
			if entry == "library" {
				preview, err = api.LibraryPreview(view.OpPinUpdate, []string{ix.Ref(cp.ID)})
			} else {
				preview, err = api.OpPreview(view.OpPinUpdate, []string{cp.ID})
			}
			if err != nil || !preview.Apply {
				t.Fatalf("preview: %+v %v", preview, err)
			}
			lines := strings.Join(preview.Lines, "\n")
			realTarget, _ := source.RealPath(target)
			realUpstream, _ := source.RealPath(filepath.Join(e.Workshop, "1001"))
			for _, want := range []string{view.T("Changes made in this copy will be overwritten and lost. The old folder goes to the recycle bin."), realTarget, realUpstream} {
				if !strings.Contains(strings.ToLower(lines), strings.ToLower(want)) {
					t.Fatalf("missing %q in %s", want, lines)
				}
			}
			if _, err := os.Stat(filepath.Join(target, "local.txt")); err != nil {
				t.Fatal("preview wrote files", err)
			}
			if entry == "library" {
				_, err = api.LibraryDo(view.OpPinUpdate, []string{ix.Ref(cp.ID)})
			} else {
				_, err = api.RunOp(view.OpPinUpdate, []string{cp.ID})
			}
			if err != nil {
				t.Fatal(err)
			}
			task := endedTask(t, api, view.OpPinUpdate)
			if task.State != app.TaskDone {
				t.Fatalf("task: %+v", task)
			}
			if _, err := os.Stat(filepath.Join(target, "local.txt")); !os.IsNotExist(err) {
				t.Fatal("local edits remain", err)
			}
			ix, err = e.App.Index()
			if err != nil || ix.ByID[cp.ID] == nil || ix.ByID[cp.ID].LineageID != cp.LineageID {
				t.Fatal("identity lost", err)
			}
			// A stale menu request is revalidated before offering confirmation.
			e.Remove(filepath.Join(e.Workshop, "1001"))
			e.MustRun("scan")
			preview, err = api.LibraryPreview(view.OpPinUpdate, []string{ix.Ref(cp.ID)})
			if err != nil || preview.Apply {
				t.Fatalf("missing upstream preview: %+v %v", preview, err)
			}
		})
	}
}
