package cmds_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestSubscribeCommands(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	g := testkit.NewGitRepo(t, "")
	for _, args := range [][]string{
		{"subscribe", "git", g.Bare, "--branch", "main", "--tag", "v1"},
		{"subscribe", "git", g.Bare, "--commit", "abcd"},
		{"subscribe", "git", g.Bare, "--tag", ""},
		{"subscribe", "remove"},
	} {
		if out, code := e.Run(args...); code == 0 {
			t.Fatalf("accepted %v: %s", args, out)
		}
	}
	if _, err := os.Stat(e.App.SubscribedDir()); !os.IsNotExist(err) {
		t.Fatal("invalid commands wrote files")
	}
	dir := filepath.Join(e.Root, "custom-subscriptions")
	e.MustRun("config", "set", "subscribed.dir", dir)
	if e.App.SubscribedDir() != dir {
		t.Fatal("subscribed.dir not applied")
	}
	e.MustRun("subscribe", "git", g.Bare, "--branch", "main")
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 1 {
		t.Fatalf("installed: %v %v", ss, err)
	}
	ref := ss[0].Ref
	if out := e.MustRun("subscribe", "list"); !strings.Contains(out, ref) || !strings.Contains(out, "not checked") {
		t.Fatal(out)
	}
	if out := e.MustRun("subscribe", "check"); !strings.Contains(out, "up to date") {
		t.Fatal(out)
	}
	if out := e.MustRun("subscribe", "update", "--dry-run"); !strings.Contains(out, "dry run, nothing changed") || !strings.Contains(out, g.First) {
		t.Fatal(out)
	}
	if out := e.MustRun("subscribe", "remove", ref, "--dry-run"); !strings.Contains(out, "dry run, nothing changed") {
		t.Fatal(out)
	}
	if _, err := os.Stat(ss[0].Folder); err != nil {
		t.Fatal("remove dry run touched folder")
	}
}
