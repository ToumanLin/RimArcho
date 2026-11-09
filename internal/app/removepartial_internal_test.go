package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// partialFixture is one game whose Mods/ links to a Mod folder with two files.
func partialFixture(t *testing.T) (a *App, target, link string) {
	t.Helper()
	root := t.TempDir()
	a, err := Open(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	a.LockBase = filepath.Join(root, "local")
	a.GameProbe = func(string) *GameRun { return nil }
	target = filepath.Join(root, "Library", "Mod")
	for _, f := range []string{"About/About.xml", "Defs/a.xml"} {
		p := filepath.Join(target, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	game := filepath.Join(root, "game")
	mods := filepath.Join(game, "Mods")
	if err := os.MkdirAll(mods, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.InsertEnv(&env.Environment{ID: "g", Name: "g", GameRoot: game}); err != nil {
		t.Fatal(err)
	}
	if err := a.applyLinks(mods, []LinkChange{{Op: "add", Name: "_Archo_env_mod", Target: target, Mod: "mod"}}); err != nil {
		t.Fatal(err)
	}
	return a, target, filepath.Join(mods, "_Archo_env_mod")
}

func settledLinks(t *testing.T, a *App) []store.ManagedLink {
	t.Helper()
	rows, err := a.Store.ManagedLinks()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if !store.LinkFinished(r.Phase) {
			t.Fatalf("%s left in phase %q", r.Name, r.Phase)
		}
	}
	if rec, err := a.linkRecoveries(rows); err != nil || len(rec) != 0 {
		t.Fatalf("recoveries %+v, %v", rec, err)
	}
	return rows
}

// A deletion that fails before removing anything links the game back; one that removed
// part of the folder leaves the link removed, so the game cannot load what is left, and
// closes the ledger (09 §3).
func TestPartialDeletionKeepsLinksRemoved(t *testing.T) {
	t.Parallel()
	t.Run("nothing removed", func(t *testing.T) {
		a, target, link := partialFixture(t)
		refused := errors.New("refused")
		err := a.withIncomingLinks(target, "", func() error { return refused })
		if !errors.Is(err, refused) {
			t.Fatal(err)
		}
		if info, err := source.ReadLink(link); err != nil || info.State != source.TargetPresent {
			t.Fatal("link not put back", info, err)
		}
		settledLinks(t, a)
	})
	t.Run("part removed", func(t *testing.T) {
		a, target, link := partialFixture(t)
		cut := errors.New("access denied")
		err := a.withIncomingLinks(target, "", func() error {
			if err := os.Remove(filepath.Join(target, "Defs", "a.xml")); err != nil {
				t.Fatal(err)
			}
			return &partlyRemovedError{path: target, err: cut}
		})
		if !errors.Is(err, cut) {
			t.Fatal(err)
		}
		if _, err := os.Lstat(link); !os.IsNotExist(err) {
			t.Fatal("game linked to a half-deleted Mod", err)
		}
		if _, err := os.Stat(filepath.Join(target, "About", "About.xml")); err != nil {
			t.Fatal("what was left was touched", err)
		}
		settledLinks(t, a)
	})
}

func TestRemoveModTreeReportsPartialRemoval(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone")
	if err := removeModTree(missing); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "file.txt")
	os.WriteFile(file, []byte("x"), 0o644)
	if err := removeModTree(file); err != nil {
		t.Fatal(err)
	}
	whole := filepath.Join(dir, "whole")
	os.MkdirAll(filepath.Join(whole, "a"), 0o755)
	os.WriteFile(filepath.Join(whole, "a", "b.txt"), []byte("x"), 0o644)
	if err := removeModTree(whole); err != nil || dirExists(whole) {
		t.Fatal(err)
	}
}
