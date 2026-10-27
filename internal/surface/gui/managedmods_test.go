package gui

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Both detail entrypoints must offer the same disk and identity actions for a
// copy in another game's Mods/ as for the selected game's local folder.
func TestOtherGameCopyActions(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	game := e.OtherGame("Other", "1.6.4633 rev1261")
	from := filepath.Join(game, "Mods", "Alpha Copy")
	e.CopyTree(filepath.Join(e.Workshop, "1001"), from)
	e.CopyTree(filepath.Join(e.Workshop, "1001"), filepath.Join(game, "Mods", "Alpha Spare"))
	e.MustRun("env", "add", "old", "--game", game, "--saves", "own")
	oldEnv, _ := e.App.Store.EnvByName("old")
	if err := e.App.Store.SetCurrentEnv(oldEnv.ID); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	mainEnv, _ := e.App.Store.EnvByName("main")
	if err := e.App.Store.SetCurrentEnv(mainEnv.ID); err != nil {
		t.Fatal(err)
	}
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m := ix.ModAt(from)
	if m == nil || m.Kind != source.Elsewhere {
		t.Fatalf("missing other-game copy: %+v", m)
	}
	copies, err := api.ModCopies(m.ID)
	if err != nil || copies.Row == nil {
		t.Fatalf("copies: %+v %v", copies, err)
	}
	found := false
	for _, i := range copies.Row.Insts {
		if i.ID != m.ID {
			continue
		}
		found = true
		for _, op := range []string{view.OpDelete, view.OpMove, view.OpPin, view.OpSeparate} {
			if !slices.Contains(i.Ops, op) {
				t.Errorf("missing %s in %v", op, i.Ops)
			}
		}
	}
	if !found {
		t.Fatal("other-game copy missing from GUI")
	}
	detail, err := api.Detail(m.ID, false, false, false)
	if err != nil || !detail.CanSeparate {
		t.Fatalf("detail: %+v %v", detail, err)
	}
	if _, err := api.LibraryDo(view.OpSeparate, []string{ix.Ref(m.ID)}); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.SetCurrentEnv(oldEnv.ID); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	ix, err = e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	split := ix.ByID[m.ID]
	if split == nil || split.LineageID == m.LineageID {
		t.Fatal("not-a-copy did not persist through owner scan")
	}
	if err := e.App.Store.SetCurrentEnv(mainEnv.ID); err != nil {
		t.Fatal(err)
	}
	// A deliberately separated lineage can still move out of a game-local
	// packageId collision, just as a copy in the selected game's Mods/ can.
	if _, err := e.App.MoveToLocked(ix.Ref(m.ID), true); err != nil {
		t.Fatal("separated other-game duplicate cannot move", err)
	}
}
