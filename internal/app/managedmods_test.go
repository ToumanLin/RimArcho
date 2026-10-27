package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// The index projects another game's Local as Elsewhere. File operations must
// retain its ownership, instance identity and every incoming deployment link.
func TestOtherGamesManagedMods(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"trash", "move", "copy", "legacy-lock", "update"} {
		t.Run(op, func(t *testing.T) {
			e := libFixture(t)
			e.App.GameProbe = func(string) *app.GameRun { return nil }
			game := secondGame(e, "Other")
			from := filepath.Join(game, "Mods", "Spare")
			e.CopyTree(filepath.Join(e.Workshop, "5100"), from)
			e.MustRun("env", "add", "old", "--game", game, "--saves", "own")
			mainEnv, _ := e.App.Env()
			oldEnv, _ := e.App.Store.EnvByName("old")
			if err := e.App.Store.SetCurrentEnv(oldEnv.ID); err != nil {
				t.Fatal(err)
			}
			scanNow(t, e)
			id := refID(t, e, "Lockable", source.Local)
			ix, _ := e.App.Index()
			ownerID := ix.Env.ID
			if op == "legacy-lock" || op == "update" {
				origin := `{"origin_instance":"` + refID(t, e, "Lockable", source.Workshop) + `","origin_snapshot":"old"}`
				e.WriteFile(filepath.Join(from, ".rimarcho", "origin.json"), origin)
				scanNow(t, e)
			}
			if err := e.App.Store.SetCurrentEnv(mainEnv.ID); err != nil {
				t.Fatal(err)
			}
			ref := "#" + id[:6]
			lib := mustLib(t, e)
			inst := lib.Instance(id)
			if inst == nil || inst.Mod.Kind != source.Elsewhere || !inst.Spare {
				t.Fatalf("other-game spare: %+v", inst)
			}
			if op == "legacy-lock" || op == "update" {
				if inst.Locked == nil {
					t.Fatal("other-game origin marker not recognized")
				}
			}
			switch op {
			case "trash":
				here := refOf(t, e, "Lockable", source.Local)
				e.Remove(filepath.Join(e.Game, "Mods", "Lockable Copy"))
				scanNow(t, e)
				e.MustRun("swap", here, ref)
				e.MustRun("save")
				calls := 0
				e.App.TrashFunc = func(path string) error {
					calls++
					if path != mustReal(t, from) {
						t.Fatalf("wrong trash target: %s", path)
					}
					return os.RemoveAll(path)
				}
				preview, err := e.App.Trash(ref, true)
				if err != nil || len(preview.UsedBy) == 0 || calls != 0 {
					t.Fatalf("preview: %+v %v", preview, err)
				}
				if _, err := os.Stat(from); err != nil {
					t.Fatal(err)
				}
				if _, err := e.App.Trash(ref, false); err != nil {
					t.Fatal(err)
				}
				if calls != 1 {
					t.Fatalf("trash calls: %d", calls)
				}
				mods, err := e.App.Store.Mods(ownerID)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, m := range mods {
					if m.ID == id {
						found = true
						if m.Status == identity.Live {
							t.Fatal("trashed other-game copy still live")
						}
					}
				}
				if !found {
					t.Fatal("trashed copy lost its tombstone")
				}
				st, err := e.App.Loadout()
				if err != nil || st.List.Index(id) < 0 {
					t.Fatalf("plan lost missing entry: %v", err)
				}
			case "move", "legacy-lock":
				// An installation in a third game must follow the moved folder.
				third := secondGame(e, "Third")
				e.MustRun("env", "add", "third", "--game", third)
				link := filepath.Join(third, "Mods", "_Archo_env_"+id)
				e.Junction(link, from)
				move := e.App.MoveToLocked
				if op == "legacy-lock" {
					move = e.App.MoveToLibrary
				}
				preview, err := move(ref, true)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(from); err != nil {
					t.Fatal("preview moved source", err)
				}
				moved, err := move(ref, false)
				if err != nil {
					t.Fatal(err)
				}
				if moved.After == nil || moved.After.ID != id || moved.After.LineageID != inst.Mod.LineageID || moved.To != preview.To {
					t.Fatalf("move changed identity: %+v", moved)
				}
				if _, err := os.Stat(from); !os.IsNotExist(err) {
					t.Fatal("original remains", err)
				}
				if mustReal(t, link) != mustReal(t, moved.To) {
					t.Fatal("incoming link did not follow move")
				}
			case "copy":
				preview, err := e.App.Pin(ref, "", false, true)
				if err != nil || preview.Copy.Files == 0 {
					t.Fatalf("copy preview: %+v %v", preview, err)
				}
				pinned, err := e.App.Pin(ref, "", false, false)
				if err != nil {
					t.Fatal(err)
				}
				if pinned.CopyMod == nil || pinned.CopyMod.ID == id || pinned.CopyMod.LineageID != inst.Mod.LineageID {
					t.Fatalf("copy: %+v", pinned)
				}
				if _, err := os.Stat(from); err != nil {
					t.Fatal("copy removed original", err)
				}
			case "update":
				updated, err := e.App.PinUpdate(ref, "", false, false)
				if err != nil || updated.Moved == nil || updated.Moved.After.ID != id || updated.CopyMod == nil {
					t.Fatalf("update: %+v %v", updated, err)
				}
			}
			current, err := e.App.Env()
			if err != nil || current.Name != "main" {
				t.Fatalf("operation changed selected game: %+v %v", current, err)
			}
		})
	}
}

func TestTrashRegisteredExtraFolder(t *testing.T) {
	t.Parallel()
	e := libFixture(t)
	root := filepath.Join(e.Root, "ExtraMods")
	from := filepath.Join(root, "ExtraCopy")
	e.CopyTree(filepath.Join(e.Workshop, "5100"), from)
	if _, err := e.App.FolderAdd(source.Extra, root); err != nil {
		t.Fatal(err)
	}
	scanNow(t, e)
	ref := refOf(t, e, "Lockable", source.Extra)
	called := false
	e.App.TrashFunc = func(path string) error { called = true; return os.RemoveAll(path) }
	if _, err := e.App.Trash(ref, true); err != nil || called {
		t.Fatalf("preview: %v", err)
	}
	if _, err := e.App.Trash(ref, false); err != nil || !called {
		t.Fatalf("trash: %v", err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatal("extra copy remains", err)
	}
}

func TestManagedOwnershipKeepsFileOpBoundaries(t *testing.T) {
	t.Parallel()
	e := libFixture(t)
	for _, ref := range []string{refOf(t, e, "Core", source.Official), refOf(t, e, "Lockable", source.Workshop)} {
		if _, err := e.App.Trash(ref, true); !errors.Is(err, app.ErrNotTrashable) {
			t.Fatalf("protected copy %s: %v", ref, err)
		}
	}
	// Check the registered junction before following it to an external folder.
	outside := filepath.Join(e.Root, "OutsideCopy")
	e.CopyTree(filepath.Join(e.Workshop, "5100"), outside)
	e.Junction(filepath.Join(e.Game, "Mods", "LinkedCopy"), outside)
	scanNow(t, e)
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m := ix.ModAt(outside)
	if m == nil {
		t.Fatal("linked copy not indexed")
	}
	if _, err := e.App.Trash(ix.Ref(m.ID), true); !errors.Is(err, app.ErrNotTrashable) || !strings.Contains(err.Error(), "link") {
		t.Fatalf("trash link: %v", err)
	}
	if _, err := e.App.MoveToLocked(ix.Ref(m.ID), true); err == nil || !strings.Contains(err.Error(), "link") {
		t.Fatalf("move link: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("link target changed", err)
	}
}
