package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestUpdateToWorkshop(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"locked", "local", "other-game", "extra", "hidden-upstream", "steamcmd-upstream"} {
		t.Run(kind, func(t *testing.T) {
			e, ws := pinFixture(t)
			e.App.GameProbe = func(string) *app.GameRun { return nil }
			target := filepath.Join(e.Game, "Mods", "Local Copy")
			if kind == "locked" {
				e.MustRun("pin", refOf(t, e, "Lockable", source.Workshop))
				target = filepath.Join(library(e), "5100_local")
			} else {
				switch kind {
				case "other-game":
					game := secondGame(e, "Other")
					e.MustRun("env", "add", "other", "--game", game, "--saves", "own")
					target = filepath.Join(game, "Mods", "Local Copy")
				case "extra":
					root := filepath.Join(e.Root, "Extra")
					target = filepath.Join(root, "Local Copy")
					e.CopyTree(ws, target)
					if _, err := e.App.FolderAdd(source.Extra, root); err != nil {
						t.Fatal(err)
					}
				}
				if kind != "extra" {
					e.CopyTree(ws, target)
				}
				if kind == "other-game" {
					main, _ := e.App.Env()
					other, _ := e.App.Store.EnvByName("other")
					if err := e.App.Store.SetCurrentEnv(other.ID); err != nil {
						t.Fatal(err)
					}
					scanNow(t, e)
					if err := e.App.Store.SetCurrentEnv(main.ID); err != nil {
						t.Fatal(err)
					}
				}
				scanNow(t, e)
			}
			ix, err := e.App.Index()
			if err != nil {
				t.Fatal(err)
			}
			cp := ix.ModAt(target)
			if cp == nil {
				t.Fatal("copy not indexed")
			}
			beforeID, beforeLineage := cp.ID, cp.LineageID
			if err := e.App.Store.SetCopyNote(cp.ID, "my copy note", e.App.Now()); err != nil {
				t.Fatal(err)
			}
			before, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			// The upstream itself is unchanged: this must still discard local edits.
			e.WriteFile(filepath.Join(target, "Defs", "a.xml"), "<Defs><ThingDef><defName>LocalEdit</defName></ThingDef></Defs>")
			e.WriteFile(filepath.Join(target, "extra.txt"), "local only")
			if kind == "hidden-upstream" {
				e.Move(filepath.Join(ws, "About", "About.xml"), filepath.Join(ws, "About", "About.xml.disabled"))
			}
			if kind == "steamcmd-upstream" {
				root := filepath.Join(e.Root, "SteamCMDMods")
				e.CopyTree(ws, filepath.Join(root, "5100"))
				if _, err := e.App.FolderAdd(source.SteamCMD, root); err != nil {
					t.Fatal(err)
				}
				e.Remove(ws)
			}
			scanNow(t, e)
			ix, _ = e.App.Index()
			ref := ix.Ref(beforeID)
			// An installation in another game must point at the refreshed contents.
			third := secondGame(e, "Third")
			e.MustRun("env", "add", "third", "--game", third)
			link := filepath.Join(third, "Mods", "_Archo_env_"+beforeID)
			e.Junction(link, target)
			recycled := 0
			e.App.TrashFunc = func(path string) error {
				recycled++
				if !strings.EqualFold(path, mustReal(t, target)) {
					t.Fatalf("wrong trash path: %s", path)
				}
				b, err := os.ReadFile(filepath.Join(path, "extra.txt"))
				if err != nil || string(b) != "local only" {
					t.Fatal("original not retained until recycling")
				}
				return os.RemoveAll(path)
			}
			preview, err := e.App.UpdateToWorkshop(ref, true)
			if err != nil || preview.Copy.Files == 0 || preview.Copy.Bytes == 0 || recycled != 0 {
				t.Fatalf("preview: %+v %v", preview, err)
			}
			if _, err := os.Stat(filepath.Join(target, "extra.txt")); err != nil {
				t.Fatal("dry run changed files", err)
			}
			result, err := e.App.UpdateToWorkshop(ref, false)
			if err != nil || recycled != 1 {
				t.Fatalf("update: %+v %v", result, err)
			}
			after, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			fresh := after.Ix.ByID[beforeID]
			if fresh == nil || fresh.Status != "live" || fresh.LineageID != beforeLineage || !strings.EqualFold(fresh.RealPath, mustReal(t, target)) {
				t.Fatalf("identity changed: %+v", fresh)
			}
			if !reflect.DeepEqual(before.List, after.List) {
				t.Fatal("changed the plan")
			}
			notes, err := e.App.Store.CopyNotes(after.Ix.Source(fresh.SourceID).EnvID)
			if err != nil || notes[beforeID] == nil || notes[beforeID].Text != "my copy note" {
				t.Fatalf("copy note lost: %+v %v", notes[beforeID], err)
			}
			if _, err := os.Stat(filepath.Join(target, "extra.txt")); !os.IsNotExist(err) {
				t.Fatal("stale file remains", err)
			}
			content, err := os.ReadFile(filepath.Join(target, "Defs", "a.xml"))
			if err != nil || strings.Contains(string(content), "LocalEdit") {
				t.Fatal("local edits remain", err)
			}
			if _, err := os.Stat(filepath.Join(target, "About", "About.xml")); err != nil {
				t.Fatal(err)
			}
			if mustReal(t, link) != mustReal(t, target) {
				t.Fatal("incoming link not restored")
			}
			lib := mustLib(t, e)
			locked := lib.Instance(beforeID).Locked
			if kind == "locked" && (locked == nil || locked.Updated) {
				t.Fatalf("lock baseline not refreshed: %+v", locked)
			}
			if kind != "locked" && fresh.Origin != "" {
				t.Fatal("ordinary local copy became a lock")
			}
			current, _ := e.App.Env()
			if current.Name != "main" {
				t.Fatal("changed current environment")
			}
		})
	}
}

func TestUpdateToWorkshopRefusals(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"link", "outside-link", "foreign-link", "running", "trash-fails", "no-upstream"} {
		t.Run(mode, func(t *testing.T) {
			e, ws := pinFixture(t)
			target := filepath.Join(e.Game, "Mods", "Local Copy")
			e.CopyTree(ws, target)
			scanNow(t, e)
			ix, _ := e.App.Index()
			cp := ix.ModAt(target)
			ref := ix.Ref(cp.ID)
			e.App.GameProbe = func(string) *app.GameRun { return nil }
			switch mode {
			case "link":
				e.Remove(target)
				e.Junction(target, ws)
			case "outside-link":
				e.Junction(filepath.Join(target, "External"), ws)
			case "foreign-link":
				game := secondGame(e, "Third")
				e.MustRun("env", "add", "third", "--game", game)
				e.Junction(filepath.Join(game, "Mods", "_Link_foreign"), target)
			case "running":
				e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{} }
			case "no-upstream":
				e.Remove(ws)
				scanNow(t, e)
			}
			calls := 0
			e.App.TrashFunc = func(string) error { calls++; return errors.New("recycle failed") }
			if mode != "trash-fails" {
				if _, err := e.App.UpdateToWorkshop(ref, true); err == nil {
					t.Fatal("unsafe preview accepted")
				}
			}
			if _, err := e.App.UpdateToWorkshop(ref, false); err == nil {
				t.Fatal("unsafe execution accepted")
			}
			if mode == "trash-fails" && calls != 1 || mode != "trash-fails" && calls != 0 {
				t.Fatalf("recycle calls: %d", calls)
			}
			if _, err := os.Stat(filepath.Join(target, "About", "About.xml")); err != nil {
				t.Fatal("original damaged", err)
			}
			leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".archo-update-*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("staging leftovers: %v %v", leftovers, err)
			}
		})
	}
}

// Availability uses one deterministic source when both downloads are installed.
func TestWorkshopUpdateSourcePreference(t *testing.T) {
	local := &store.Mod{ID: "local", LineageID: "family", Kind: source.Local, Status: "live"}
	steamcmd := &store.Mod{ID: "a", Kind: source.SteamCMD, Status: "live"}
	workshop := &store.Mod{ID: "b", Kind: source.Workshop, Status: "live"}
	ix := &app.Index{ByLineage: map[string][]*store.Mod{"family": {steamcmd, workshop, local}}}
	if ix.WorkshopUpdateSource(local) != workshop {
		t.Fatal("Workshop not preferred")
	}
	workshop.Status = "missing"
	if ix.WorkshopUpdateSource(local) != steamcmd {
		t.Fatal("SteamCMD fallback missing")
	}
}
