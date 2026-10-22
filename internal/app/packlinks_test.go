package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// K12: a pack's links left in Mods/ while the pack is not played are a problem of the plan
// with one fix that removes only the links (08 §5.3).
func TestPackLinksLeft(t *testing.T) {
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("pack", "build", "first")
	e.MustRun("pack", "launch", "first")
	mods := filepath.Join(e.Game, "Mods")
	ix, _ := e.App.Index()

	// The game is not running: the links would be read by a game started from Steam.
	l := e.App.LeftPackLinks(ix)
	if l == nil || len(l.Links) == 0 || l.PackNames() != "first" || l.Running {
		t.Fatalf("after the pack's game: %+v", l)
	}
	if out := e.MustRun("problems", "--code", "pack"); !strings.Contains(out, "warning pack.links-left") || !strings.Contains(out, "fix: archo pack unlink") {
		t.Fatalf("problems:\n%s", out)
	}

	// The pack is being played (its save folder on the command line): the links are in use.
	userdata := filepath.Join(e.Home, "packs", "first", "userdata")
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 7, ArgsKnown: true, SaveData: userdata} }
	if l := e.App.LeftPackLinks(ix); l != nil {
		t.Fatalf("while the pack runs: %+v", l)
	}
	// Started from Steam (the default save folder): the game runs with the pack's copies.
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 7, ArgsKnown: true} }
	if l := e.App.LeftPackLinks(ix); l == nil || !l.Running || !strings.Contains(l.Finding().Claim, "the game is running") {
		t.Fatalf("started from Steam: %+v", l)
	}
	if _, code := e.Run("pack", "unlink"); code == 0 {
		t.Fatal("unlink while a game runs must be refused")
	}

	e.App.GameProbe = func(string) *app.GameRun { return nil }
	if out := e.MustRun("pack", "unlink", "--dry-run"); !strings.Contains(out, "- !ArchoPack_first_") {
		t.Fatalf("dry run:\n%s", out)
	}
	if links(t, mods) == 0 {
		t.Fatal("the dry run removed links")
	}
	e.MustRun("pack", "unlink")
	if n := links(t, mods); n != 0 {
		t.Fatalf("%d pack links left", n)
	}
	if _, err := os.Stat(filepath.Join(e.Home, "packs", "first", "mods")); err != nil {
		t.Fatal("unlink touched the pack")
	}
	if l := e.App.LeftPackLinks(ix); l != nil {
		t.Fatalf("after unlink: %+v", l)
	}
	if out := e.MustRun("pack", "unlink"); !strings.Contains(out, "no pack links") {
		t.Fatalf("nothing to do:\n%s", out)
	}
}

func links(t *testing.T, mods string) int {
	t.Helper()
	ents, err := os.ReadDir(mods)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, de := range ents {
		if strings.HasPrefix(de.Name(), "!ArchoPack_") {
			n++
		}
	}
	return n
}
