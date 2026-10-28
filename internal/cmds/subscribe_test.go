package cmds_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
		{"subscribe", "page", "https://example.invalid/mod", "--from", ""},
		{"subscribe", "page", "https://example.invalid/mod", "--from", "mod.zip", "--git"},
		{"subscribe", "update", "--from", "mod.zip"},
		{"subscribe", "workshop", "invalid", "--steamcmd"},
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
	steamDir := filepath.Join(e.Root, "custom-steamcmd-mods")
	e.MustRun("config", "set", "steamcmd-mods.dir", steamDir)
	if e.App.SteamCMDModsDir() != steamDir {
		t.Fatal("steamcmd-mods.dir not applied")
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
	if out := e.MustRun("subscribe", "update", "--all", "--dry-run"); !strings.Contains(out, "dry run, nothing changed") || !strings.Contains(out, g.First) {
		t.Fatal(out)
	}
	if out := e.MustRun("subscribe", "remove", ref, "--dry-run"); !strings.Contains(out, "dry run, nothing changed") {
		t.Fatal(out)
	}
	if _, err := os.Stat(ss[0].Folder); err != nil {
		t.Fatal("remove dry run touched folder")
	}
}

func TestSubscribePageCommands(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	version := "1.0"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `<script type="application/ld+json">{"name":"Example Colony Tweaks","softwareVersion":%q}</script>`, version)
	}))
	defer srv.Close()
	e.App.HTTP = srv.Client()
	if out := e.MustRun("subscribe", "page", srv.URL); !strings.Contains(out, "--from") {
		t.Fatal(out)
	}
	from := filepath.Join(e.Root, "downloaded")
	e.WriteMod(from, testkit.Sample("example.cli.page", "Example Colony Tweaks"))
	e.MustRun("subscribe", "page", srv.URL, "--from", from)
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 1 {
		t.Fatalf("page install: %+v %v", ss, err)
	}
	version = "1.1"
	e.MustRun("subscribe", "check", ss[0].Ref)
	e.App.TrashFunc = func(path string) error { return os.Rename(path, filepath.Join(e.Root, "old-page")) }
	e.MustRun("subscribe", "update", ss[0].Ref, "--from", from)
	after, err := e.App.Subscriptions()
	if err != nil || len(after) != 1 || after[0].Mod.ID != ss[0].Mod.ID || after[0].Record.Installed != "1.1" {
		t.Fatalf("page update: %+v %v", after, err)
	}
}
