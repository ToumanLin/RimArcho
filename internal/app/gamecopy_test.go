package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A game copy (03 §3.5) takes the game's plain files, never RimArcho's links in Mods/,
// and adds the copy as a new game started directly.
func TestCopyGame(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	e.WriteFile(filepath.Join(e.Game, "Mods", "LocalMod", "About", "About.xml"), "<ModMetaData/>")
	linked := filepath.Join(e.Root, "elsewhere", "Linked")
	e.WriteFile(filepath.Join(linked, "About", "About.xml"), "<ModMetaData/>")
	e.Junction(filepath.Join(e.Game, "Mods", "_Archo_Linked"), linked)

	dir := filepath.Join(e.Root, "Games", "RimWorld backup")
	plan, err := e.App.PlanGameCopy(app.CopyGameSpec{From: "main", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Name != "main-copy" || plan.ModBytes == 0 || plan.Bytes == 0 {
		t.Fatalf("plan %+v", plan)
	}
	var last app.CopyGameProgress
	r, err := e.App.CopyGame(context.Background(), app.CopyGameSpec{From: "main", Dir: dir, OwnSaves: true}, func(p app.CopyGameProgress) { last = p })
	if err != nil {
		t.Fatal(err)
	}
	if r.Env.Name != "main-copy" || r.Env.Launch != "direct" || r.Env.GameRoot != dir {
		t.Fatalf("env %+v", r.Env)
	}
	if last.Bytes != plan.Bytes || last.Total != plan.Bytes || last.Files == 0 {
		t.Fatalf("progress %+v, plan %d", last, plan.Bytes)
	}
	if _, err := os.Stat(filepath.Join(dir, "Version.txt")); err != nil {
		t.Fatal("the game's files are copied:", err)
	}
	if ents, _ := os.ReadDir(filepath.Join(dir, "Mods")); len(ents) != 0 {
		t.Fatalf("without Mods, Mods/ is empty: %v", ents)
	}
	if _, err := os.Stat(dir + ".archo-tmp"); !os.IsNotExist(err) {
		t.Fatal("the temporary folder is gone")
	}

	// with Mods: the local Mod comes along, RimArcho's link does not
	dir2 := filepath.Join(e.Root, "Games", "second")
	if _, err := e.App.CopyGame(context.Background(), app.CopyGameSpec{From: "main", Dir: dir2, Mods: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir2, "Mods", "LocalMod", "About", "About.xml")); err != nil {
		t.Fatal("the local Mod is copied:", err)
	}
	if _, err := os.Lstat(filepath.Join(dir2, "Mods", "_Archo_Linked")); !os.IsNotExist(err) {
		t.Fatal("RimArcho's link is not copied")
	}
	if e2, _ := e.App.Store.EnvByName("main-copy-2"); e2 == nil {
		t.Fatal("the second copy is named main-copy-2")
	}
}

func TestCopyGameRefuses(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	full := filepath.Join(e.Root, "full")
	e.WriteFile(filepath.Join(full, "x.txt"), "x")
	for _, c := range []struct {
		spec app.CopyGameSpec
		want string
	}{
		{app.CopyGameSpec{From: "nope", Dir: filepath.Join(e.Root, "a")}, "no game named"},
		{app.CopyGameSpec{From: "main", Dir: full}, "not empty"},
		{app.CopyGameSpec{From: "main", Dir: filepath.Join(e.Game, "inside")}, ""},
		{app.CopyGameSpec{From: "main", Dir: filepath.Join(e.Root, "b"), Name: "main"}, "already added"},
	} {
		_, err := e.App.PlanGameCopy(c.spec)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v", c.spec, err)
		}
	}
	// a stopped copy leaves nothing
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := filepath.Join(e.Root, "stopped")
	if _, err := e.App.CopyGame(ctx, app.CopyGameSpec{From: "main", Dir: dir}, nil); err != app.ErrCopyStopped {
		t.Fatalf("stopped: %v", err)
	}
	if _, err := os.Stat(dir + ".archo-tmp"); !os.IsNotExist(err) {
		t.Fatal("no temporary folder after a stop")
	}
	if e2, _ := e.App.Store.EnvByName("main-copy"); e2 != nil {
		t.Fatal("a stopped copy adds no game")
	}
}

func TestEnvCopyCommand(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	dir := filepath.Join(e.Root, "backup")
	out := e.MustRun("env", "copy", "main", "--to", dir, "--dry-run")
	if !strings.Contains(out, "would be copied") || !strings.Contains(out, "env:main-copy") {
		t.Fatalf("dry run:\n%s", out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("the dry run writes nothing")
	}
	out = e.MustRun("env", "copy", "main", "--to", dir, "--name", "backup", "--saves", "own")
	if !strings.Contains(out, "env:backup  copied and added") || !strings.Contains(out, "direct") {
		t.Fatalf("copy:\n%s", out)
	}
}
