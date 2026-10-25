package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestLinkLifecycleRacePreflight(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a, err := Open(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.LockBase = filepath.Join(root, "local")
	a.GameProbe = func(string) *GameRun { return nil }
	target := filepath.Join(root, "target")
	other := filepath.Join(root, "other")
	for _, p := range []string{target, other} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}
	var paths []string
	var rows []store.ManagedLink
	for _, name := range []string{"one", "two"} {
		game := filepath.Join(root, name)
		mods := filepath.Join(game, "Mods")
		if err := os.MkdirAll(mods, 0755); err != nil {
			t.Fatal(err)
		}
		if err := a.Store.InsertEnv(&env.Environment{ID: name, Name: name, GameRoot: game}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(mods, "_Archo_env_test")
		if err := source.CreateLink(path, target); err != nil {
			t.Fatal(err)
		}
		info, err := source.ReadLink(path)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
		rows = append(rows, store.ManagedLink{Operation: "fixture", Mods: realOrAbs(mods), Name: filepath.Base(path), Kind: info.Kind, Instance: "test", NextPath: filepath.Join(realOrAbs(mods), filepath.Base(path)), NextTarget: target})
	}
	if err := a.Store.RecordLinkIntent(rows); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.LinkPhase("fixture", "complete"); err != nil {
		t.Fatal(err)
	}
	p, err := a.PlanIncomingLinks(target, filepath.Join(root, "destination"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Links) != 2 {
		t.Fatalf("links %+v", p.Links)
	}
	if err := source.RemoveLink(paths[1]); err != nil {
		t.Fatal(err)
	}
	if err := source.CreateLink(paths[1], other); err != nil {
		t.Fatal(err)
	}
	if err := a.beginLinkLifecycle(p); err == nil {
		t.Fatal("changed link passed preflight")
	}
	for i, path := range paths {
		info, err := source.ReadLink(path)
		if err != nil || info.State != source.TargetPresent {
			t.Fatalf("link %d changed: %+v %v", i, info, err)
		}
	}
	if !dirExists(target) || !dirExists(other) {
		t.Fatal("race touched target")
	}
	ledger, err := a.Store.ManagedLinks()
	if err != nil || len(ledger) != 2 {
		t.Fatalf("failed preflight journalled intent: %+v %v", ledger, err)
	}
}
