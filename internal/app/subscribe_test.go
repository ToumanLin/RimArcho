package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	git "github.com/go-git/go-git/v5"
)

func gitHead(t *testing.T, dir string) string {
	t.Helper()
	r, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	h, err := r.Head()
	if err != nil {
		t.Fatal(err)
	}
	return h.Hash().String()
}

// A33: each tracking mode installs one complete Git mod and registers it immediately.
func TestSubscribeGit(t *testing.T) {
	for _, typ := range []string{"branch", "tag", "commit"} {
		t.Run(typ, func(t *testing.T) {
			e := setup(t, false)
			g := testkit.NewGitRepo(t, "")
			ref := map[string]string{"branch": "main", "tag": "v1", "commit": g.First}[typ]
			out := e.MustRun("subscribe", "git", g.Bare, "--"+typ, ref, "--dry-run")
			if !strings.Contains(out, "dry run, nothing changed") {
				t.Fatal(out)
			}
			if _, err := os.Stat(e.App.SubscribedDir()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("dry run created subscribed root: %v", err)
			}
			op, err := e.App.SubscribeGit(g.Bare, typ, ref, false)
			if err != nil {
				t.Fatal(err)
			}
			it := op.Items[0]
			if it.ID == "" || it.Ref == "" || filepath.Dir(it.Folder) != e.App.SubscribedDir() {
				t.Fatalf("installed: %+v", it)
			}
			for _, path := range []string{".git", subscribe.File, "About/About.xml"} {
				if _, err := os.Stat(filepath.Join(it.Folder, path)); err != nil {
					t.Fatal(err)
				}
			}
			ss, err := e.App.Subscriptions()
			if err != nil || len(ss) != 1 {
				t.Fatalf("subscriptions: %+v %v", ss, err)
			}
			r := ss[0].Record
			if r.Kind != "git" || r.URL != g.Bare || r.RefType != typ || r.Ref != ref || r.Installed != g.First || !r.InstalledAt.Equal(e.Now()) {
				t.Fatalf("record: %+v", r)
			}
			if ms := named(t, e, "Git Mod"); len(ms) != 1 || ms[0].ID != it.ID || ms[0].SourceLabel != app.SubscribedLabel || ms[0].Kind != source.Extra {
				t.Fatalf("instances: %+v", ms)
			}
			scanNow(t, e)
			if len(named(t, e, "Git Mod")) != 1 {
				t.Fatal("rescan duplicated instance")
			}
			if _, err := e.App.SubscribeGit(g.Bare, typ, ref, false); err == nil || !strings.Contains(err.Error(), "subscribed already") {
				t.Fatalf("duplicate: %v", err)
			}
			if _, err := e.App.FolderRemove("src:subscribed"); err == nil {
				t.Fatal("removed managed source")
			}
			if d := directory(t, e.App, "subscribed"); d.Path != e.App.SubscribedDir() || d.Status != "ok" {
				t.Fatalf("directory: %+v", d)
			}
			if out := e.MustRun("subscribe", "list"); !strings.Contains(out, "1 subscriptions") || !strings.Contains(out, g.First) {
				t.Fatal(out)
			}
		})
	}
	t.Run("default and collision", func(t *testing.T) {
		e := setup(t, false)
		g := testkit.NewGitRepo(t, "")
		e.MkdirAll(filepath.Join(e.App.SubscribedDir(), "mod"))
		e.WriteFile(filepath.Join(e.App.SubscribedDir(), "mod", "keep.txt"), "occupied")
		op, err := e.App.SubscribeGit(g.Bare, "branch", "", false)
		if err != nil || filepath.Base(op.Items[0].Folder) != "mod-2" {
			t.Fatalf("collision: %+v %v", op, err)
		}
		if _, err := os.Stat(filepath.Join(e.App.SubscribedDir(), "mod", "keep.txt")); err != nil {
			t.Fatal(err)
		}
		// A temporary mod must never become an instance, even on an explicit full scan.
		e.WriteFile(filepath.Join(e.App.SubscribedDir(), ".archo-tmp-test", "About", "About.xml"), `<ModMetaData><name>Incomplete</name><packageId>tester.incomplete</packageId></ModMetaData>`)
		scanNow(t, e)
		if len(named(t, e, "Incomplete")) != 0 {
			t.Fatal("scanned temporary clone")
		}
	})
	t.Run("non-mod root", func(t *testing.T) {
		e := setup(t, false)
		g := testkit.NewGitRepo(t, "NestedMod")
		if _, err := e.App.SubscribeGit(g.Bare, "branch", "", false); err == nil || !strings.Contains(err.Error(), "NestedMod") || !strings.Contains(err.Error(), "root has no About/About.xml") {
			t.Fatalf("nested mod: %v", err)
		}
		entries, err := os.ReadDir(e.App.SubscribedDir())
		if err != nil || len(entries) != 0 {
			t.Fatalf("failed clone remains: %v %v", entries, err)
		}
		if len(named(t, e, "Git Mod")) != 0 {
			t.Fatal("non-root mod installed")
		}
	})
}

// A34: check is persisted, dry run is inert, update keeps identity and player edits win.
func TestSubscribeGitUpdate(t *testing.T) {
	e := setup(t, false)
	g := testkit.NewGitRepo(t, "")
	op, err := e.App.SubscribeGit(g.Bare, "branch", "main", false)
	if err != nil {
		t.Fatal(err)
	}
	it := op.Items[0]
	e.MustRun("enable", it.Ref)
	if r := e.App.SubscriptionOf(one(t, e, "Git Mod")); r == nil || r.HasUpdate() {
		t.Fatalf("initial cache: %+v", r)
	}
	second := g.Commit(t, "Defs/Test.xml", "<Defs><ThingDef><defName>Second</defName></ThingDef></Defs>")
	e.Advance(time.Minute)
	out := e.MustRun("subscribe", "check", it.Ref)
	if !strings.Contains(out, "update available") {
		t.Fatal(out)
	}
	r, err := subscribe.Read(it.Folder)
	if err != nil || !r.HasUpdate() || r.Latest != second || !r.CheckedAt.Equal(e.Now()) {
		t.Fatalf("checked: %+v %v", r, err)
	}
	if got := e.App.SubscriptionOf(one(t, e, "Git Mod")); got == nil || !got.HasUpdate() {
		t.Fatalf("stale cache: %+v", got)
	}
	before, err := os.ReadFile(filepath.Join(it.Folder, subscribe.File))
	if err != nil {
		t.Fatal(err)
	}
	key := e.App.Store.ChangeKey()
	out = e.MustRun("subscribe", "update", it.Ref, "--dry-run")
	if !strings.Contains(out, g.First) || !strings.Contains(out, second) {
		t.Fatal(out)
	}
	after, _ := os.ReadFile(filepath.Join(it.Folder, subscribe.File))
	if string(before) != string(after) || e.App.Store.ChangeKey() != key || gitHead(t, it.Folder) != g.First {
		t.Fatal("dry run changed files or database")
	}
	if _, err := os.Stat(filepath.Join(it.Folder, "Defs", "Test.xml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry run fetched content")
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
	if _, err := e.App.UpdateSubscriptions(nil, false); err == nil || !strings.Contains(err.Error(), "game is running") {
		t.Fatalf("game running: %v", err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	e.MustRun("subscribe", "update", it.Ref)
	if gitHead(t, it.Folder) != second || one(t, e, "Git Mod").ID != it.ID {
		t.Fatal("update did not advance or changed identity")
	}
	if got := e.App.SubscriptionOf(one(t, e, "Git Mod")); got.Installed != second || got.HasUpdate() {
		t.Fatalf("updated cache: %+v", got)
	}
	if !contains(active(t, e), "Git Mod") {
		t.Fatal("update lost plan entry")
	}
	env, _ := e.App.Env()
	evs, err := e.App.Store.Events(store.EventFilter{EnvID: env.ID})
	if err != nil {
		t.Fatal(err)
	}
	updated := false
	for _, ev := range evs {
		if ev.Kind == "updated" && ev.InstanceID == it.ID {
			updated = true
		}
	}
	if !updated {
		t.Fatal("no updated event")
	}
	third := g.Commit(t, "Defs/Test.xml", "<Defs><ThingDef><defName>Third</defName></ThingDef></Defs>")
	e.WriteFile(filepath.Join(it.Folder, "About", "About.xml"), "player changes")
	if _, err := e.App.UpdateSubscriptions(nil, false); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("dirty worktree: %v", err)
	}
	if gitHead(t, it.Folder) != second || gitHead(t, it.Folder) == third {
		t.Fatal("dirty update changed HEAD")
	}
	if b, _ := os.ReadFile(filepath.Join(it.Folder, "About", "About.xml")); string(b) != "player changes" {
		t.Fatal("player changes lost")
	}
	// Check failure preserves the previous conclusion and records an actionable error.
	if err := os.Rename(g.Bare, g.Bare+"-offline"); err != nil {
		t.Fatal(err)
	}
	e.Advance(time.Minute)
	if _, err := e.App.CheckSubscriptions(nil); err == nil {
		t.Fatal("missing upstream check succeeded")
	}
	r, err = subscribe.Read(it.Folder)
	if err != nil || r.Latest != second || r.CheckError == "" || !r.CheckedAt.Equal(e.Now()) {
		t.Fatalf("failed check: %+v %v", r, err)
	}
}

// A36: removal warns for plans and delegates deletion to the recycle bin.
func TestSubscribeRemove(t *testing.T) {
	e := setup(t, true)
	g := testkit.NewGitRepo(t, "")
	op, err := e.App.SubscribeGit(g.Bare, "branch", "", false)
	if err != nil {
		t.Fatal(err)
	}
	it := op.Items[0]
	e.MustRun("enable", it.Ref)
	game := secondGame(e, "SubscriptionIncoming")
	e.MustRun("env", "add", "subscription-incoming", "--game", game)
	incoming := filepath.Join(game, "Mods", "_Archo_env_"+it.ID)
	if err := source.CreateLink(incoming, it.Folder); err != nil {
		t.Fatal(err)
	}
	var trashed []string
	e.App.TrashFunc = func(path string) error {
		if source.IsLink(incoming) {
			t.Fatal("subscription recycled before incoming link detached")
		}
		trashed = append(trashed, path)
		dest := filepath.Join(e.Root, "fake-trash", filepath.Base(path))
		e.MkdirAll(filepath.Dir(dest))
		return os.Rename(path, dest)
	}
	preview, err := e.App.RemoveSubscriptions([]string{it.Ref}, true)
	if err != nil || !reflect.DeepEqual(preview.Items[0].UsedBy, []string{"plan:default"}) || len(trashed) != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if _, err := e.App.RemoveSubscriptions([]string{"pkg:ludeon.rimworld"}, false); err == nil || !strings.Contains(err.Error(), "not a subscription") {
		t.Fatalf("non-subscription: %v", err)
	}
	if _, err := e.App.RemoveSubscriptions([]string{it.Ref, "pkg:ludeon.rimworld"}, false); err == nil || len(trashed) != 0 {
		t.Fatalf("partial invalid selection: %v %v", err, trashed)
	}
	if err := e.App.ToTrash(filepath.Join(it.Folder, "About")); !errors.Is(err, app.ErrNotInTrashList) {
		t.Fatalf("nested folder trash: %v", err)
	}
	out := e.MustRun("subscribe", "remove", it.Ref)
	if source.IsLink(incoming) {
		t.Fatal("subscription removal left incoming link")
	}
	if !strings.Contains(out, "plan:default") || !strings.Contains(out, "recycle bin") || len(trashed) != 1 || filepath.Base(trashed[0]) != filepath.Base(it.Folder) {
		t.Fatalf("remove: %s %v", out, trashed)
	}
	if _, err := os.Stat(it.Folder); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("subscription folder remains")
	}
	if one(t, e, "Git Mod").Status != identity.Tombstone {
		t.Fatal("removal not rescanned")
	}
	if _, err := os.Stat(filepath.Join(e.Root, "fake-trash", filepath.Base(it.Folder), subscribe.File)); err != nil {
		t.Fatal("record did not reach fake trash")
	}
	if ss, err := e.App.Subscriptions(); err != nil || len(ss) != 0 {
		t.Fatalf("removed subscriptions: %v %v", ss, err)
	}
	// Existing SteamCMD mods need no source.json to be subscriptions.
	e.WriteMod(filepath.Join(e.SteamCMD, "7500"), testkit.Sample("tester.steamcmdsub", "SteamCMD Mod"))
	scanNow(t, e)
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 1 || ss[0].Record.Kind != "steamcmd" || ss[0].Record.WorkshopID != "7500" {
		t.Fatalf("SteamCMD subscription: %+v %v", ss, err)
	}
	e.MustRun("subscribe", "remove", ss[0].Ref)
	if len(trashed) != 2 {
		t.Fatalf("SteamCMD not trashed: %v", trashed)
	}
}

func TestSubscribedDirectoryMove(t *testing.T) {
	e := setup(t, false)
	g := testkit.NewGitRepo(t, "")
	op, err := e.App.SubscribeGit(g.Bare, "branch", "", false)
	if err != nil {
		t.Fatal(err)
	}
	old := e.App.SubscribedDir()
	to := filepath.Join(e.Root, "moved-subscriptions")
	if _, err := e.App.MoveDirectory("subscribed", to); err != nil {
		t.Fatal(err)
	}
	if e.App.SubscribedDir() != to {
		t.Fatal("move did not set subscribed.dir")
	}
	scanNow(t, e)
	realTo, _ := source.RealPath(to)
	if m := one(t, e, "Git Mod"); m.ID != op.Items[0].ID || !source.Within(m.RealPath, realTo) {
		t.Fatalf("moved instance: %+v", m)
	}
	if err := os.Rename(to, old); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.SubscribeGit(g.Bare, "tag", "v1", false); err == nil {
		t.Fatal("recreated missing managed root")
	}
	if err := e.App.SetDirectory("subscribed", old); err != nil {
		t.Fatal(err)
	}
	scanNow(t, e)
	if r := e.App.SubscriptionOf(one(t, e, "Git Mod")); r == nil || r.Installed != g.First {
		t.Fatalf("moved record: %+v", r)
	}
}
