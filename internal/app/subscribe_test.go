package app_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

func TestUpdateWorkshopGroups(t *testing.T) {
	t.Parallel()
	e := setup(t, true)
	roots := []string{
		filepath.Join(e.Root, "batch library A", "steamapps", "workshop", "content", "294100"),
		filepath.Join(e.Root, "batch library B", "steamapps", "workshop", "content", "294100"),
	}
	for _, root := range roots {
		for i := 1; i <= 27; i++ {
			id := fmt.Sprint(8000 + i)
			e.WriteMod(filepath.Join(root, id), testkit.Sample("group.mod"+id, "Group "+id))
		}
		if _, err := e.App.FolderAdd(source.SteamCMD, root); err != nil {
			t.Fatal(err)
		}
	}
	scanNow(t, e)
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 54 {
		t.Fatalf("initial subscriptions: %d %v", len(ss), err)
	}
	var refs []string
	// Interleave roots: each root must remain its own queue, in first-seen order.
	for i := 1; i <= 27; i++ {
		for _, root := range roots {
			for _, sub := range ss {
				realRoot, err := source.RealPath(root)
				if err != nil {
					t.Fatal(err)
				}
				if strings.EqualFold(filepath.Base(sub.Folder), fmt.Sprint(8000+i)) && strings.EqualFold(filepath.Dir(sub.Folder), realRoot) {
					refs = append(refs, sub.Ref)
				}
			}
		}
	}
	if len(refs) != 54 {
		t.Fatalf("references: %v", refs)
	}
	// Managed incoming links survive a partial failure in the same source.
	incoming := filepath.Join(e.Game, "Mods", "_Archo_group-test")
	target := filepath.Join(roots[0], "8001")
	if err := source.CreateLink(incoming, target); err != nil {
		t.Fatal(err)
	}
	e.App.FindTool = func(string) (string, bool) { return filepath.Join(e.Root, "steamcmd.exe"), true }
	var calls [][]string
	var bases []string
	e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
		ids := workshopScriptIDs(t, exe, args)
		calls = append(calls, ids)
		bases = append(bases, args[1])
		if args[3] != "anonymous" {
			t.Fatalf("effective account: %v", args)
		}
		if source.IsLink(incoming) && len(calls) <= 2 {
			t.Fatal("incoming link not detached during group")
		}
		var out []string
		for _, id := range ids {
			if id == "8002" && len(calls) == 1 {
				out = append(out, "ERROR! Download item "+id+" failed (AccessDenied).")
				continue
			}
			p := filepath.Join(args[1], "steamapps", "workshop", "content", "294100", id)
			e.WriteFile(filepath.Join(p, "updated.xml"), "<Defs/>")
			out = append(out, "Success. Downloaded item "+id)
		}
		return strings.Join(out, "\n"), nil
	}
	var before int
	if err := e.App.Store.DB.QueryRow("SELECT count(*) FROM scan").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.UpdateSubscriptions(refs, true); err != nil || len(calls) != 0 {
		t.Fatalf("dry run: %v", err)
	}
	// A background task reports on each of its items (16 §2.16) while the downloads stay grouped.
	started, ended := map[int]bool{}, map[int]error{}
	op, err := e.App.UpdateSubscriptionsWith(refs, false, app.BatchCalls{
		Start: func(i int) { started[i] = true },
		Done: func(i int, err error) {
			if !started[i] {
				t.Errorf("item %d ended before it started", i)
			}
			if _, twice := ended[i]; twice {
				t.Errorf("item %d ended twice", i)
			}
			ended[i] = err
		},
	})
	if err == nil || !strings.Contains(err.Error(), "8002") {
		t.Fatalf("partial failure: %v", err)
	}
	if len(ended) != len(refs) {
		t.Fatalf("reported %d of %d items", len(ended), len(refs))
	}
	for i, err := range ended {
		if (i == 2) != (err != nil) {
			t.Fatalf("item %d reported %v", i, err)
		}
	}
	if len(calls) != 4 || len(calls[0]) != 25 || len(calls[1]) != 2 || len(calls[2]) != 25 || len(calls[3]) != 2 || bases[0] != bases[1] || bases[2] != bases[3] || bases[0] == bases[2] {
		t.Fatalf("groups: %v, bases %v", calls, bases)
	}
	var after int
	if err := e.App.Store.DB.QueryRow("SELECT count(*) FROM scan").Scan(&after); err != nil || after-before != 1 {
		t.Fatalf("scan delta %d: %v", after-before, err)
	}
	for i, item := range op.Items {
		if item.Ref != refs[i] || item.ID == "" {
			t.Fatalf("result order: %+v", item)
		}
		if i == 2 {
			if !strings.HasPrefix(item.Result, "failed:") || item.To != "" {
				t.Fatalf("failure: %+v", item)
			}
		} else if item.To == "" || item.Result != "downloaded with SteamCMD" {
			t.Fatalf("lost success: %+v", item)
		}
	}
	resolved, err := source.RealPath(incoming)
	want, _ := source.RealPath(target)
	if err != nil || !strings.EqualFold(resolved, want) {
		t.Fatalf("incoming link not restored: %s %v", resolved, err)
	}
}

func TestUpdateWorkshopGitOrdering(t *testing.T) {
	for _, tc := range []struct{ failingGit, failingSteam bool }{{}, {true, false}, {false, true}, {true, true}} {
		t.Run(fmt.Sprintf("git=%v/steam=%v", tc.failingGit, tc.failingSteam), func(t *testing.T) {
			failingGit := tc.failingGit
			e := setup(t, true)
			g := testkit.NewGitRepo(t, "")
			gitOp, err := e.App.SubscribeGit(g.Bare, "branch", "main", false)
			if err != nil {
				t.Fatal(err)
			}
			next := g.Commit(t, "Defs/Updated.xml", "<Defs/>")
			for _, id := range []string{"8100", "8200"} {
				e.WriteMod(filepath.Join(e.SteamCMD, id), testkit.Sample("ordering.mod"+id, id))
			}
			scanNow(t, e)
			ss, err := e.App.Subscriptions()
			if err != nil {
				t.Fatal(err)
			}
			refs := []string{"", gitOp.Items[0].Ref, ""}
			for _, sub := range ss {
				if sub.Record.WorkshopID == "8100" {
					refs[0] = sub.Ref
				}
				if sub.Record.WorkshopID == "8200" {
					refs[2] = sub.Ref
				}
			}
			if failingGit {
				e.WriteFile(filepath.Join(gitOp.Items[0].Folder, "About", "About.xml"), "player changes")
			}
			e.App.FindTool = func(string) (string, bool) { return filepath.Join(e.Root, "steamcmd.exe"), true }
			var calls [][]string
			e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
				ids := workshopScriptIDs(t, exe, args)
				calls = append(calls, ids)
				if len(calls) == 1 {
					if gitHead(t, gitOp.Items[0].Folder) != g.First {
						t.Fatal("Git ran before preceding SteamCMD group")
					}
				} else if gitHead(t, gitOp.Items[0].Folder) != next {
					t.Fatal("later SteamCMD group ran before Git")
				}
				if tc.failingSteam && len(calls) == 1 {
					return "ERROR! Download item 8100 failed (AccessDenied)", nil
				}
				p := filepath.Join(args[1], "steamapps", "workshop", "content", "294100", ids[0])
				e.WriteFile(filepath.Join(p, "updated.xml"), "<Defs/>")
				return "Success. Downloaded item " + ids[0], nil
			}
			op, err := e.App.UpdateSubscriptions(refs, false)
			if failingGit {
				if err == nil || !strings.Contains(err.Error(), "dirty") || len(calls) != 1 {
					t.Fatalf("Git error boundary: %v calls %v", err, calls)
				}
			} else if len(calls) != 2 || (err != nil) != tc.failingSteam {
				t.Fatalf("Git ordering: %v calls %v", err, calls)
			}
			if !slices.Equal(calls[0], []string{"8100"}) {
				t.Fatalf("preceding group: %+v", op)
			}
			if tc.failingSteam {
				if !strings.HasPrefix(op.Items[0].Result, "failed:") || op.Items[0].To != "" {
					t.Fatalf("preceding failure lost: %+v", op)
				}
				if !failingGit && (op.Items[1].To != next || op.Items[2].To == "" || gitHead(t, gitOp.Items[0].Folder) != next) {
					t.Fatalf("SteamCMD failure prevented later Git/workshop success: %+v", op)
				}
			} else if op.Items[0].To == "" {
				t.Fatalf("preceding success lost: %+v", op)
			} else if _, err := subscribe.Read(op.Items[0].Folder); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUpdateWorkshopGroupsAcrossSkippedPages(t *testing.T) {
	t.Parallel()
	e := setup(t, true)
	e.WriteMod(filepath.Join(e.SteamCMD, "8100"), testkit.Sample("group.first", "A Workshop"))
	e.WriteMod(filepath.Join(e.SteamCMD, "8200"), testkit.Sample("group.last", "C Workshop"))
	pageDir := filepath.Join(e.App.SubscribedDir(), "page")
	e.WriteMod(pageDir, testkit.Sample("group.page", "B Page"))
	if err := subscribe.Write(pageDir, &subscribe.Record{Kind: "page", URL: "https://www.loverslab.com/files/file/123-fixture/"}); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.InsertSource(&source.Source{ID: e.App.NewID(), EnvID: store.Shared, Kind: source.Extra, Label: app.SubscribedLabel, Root: e.App.SubscribedDir(), Status: source.StatusOK}); err != nil {
		t.Fatal(err)
	}
	scanNow(t, e)
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 3 || ss[0].Record.Kind != "steamcmd" || ss[1].Record.Kind != "page" || ss[2].Record.Kind != "steamcmd" {
		t.Fatalf("ordering fixture: %+v %v", ss, err)
	}
	e.App.FindTool = func(string) (string, bool) { return filepath.Join(e.Root, "steamcmd.exe"), true }
	var calls [][]string
	e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
		ids := workshopScriptIDs(t, exe, args)
		calls = append(calls, ids)
		var out []string
		for _, id := range ids {
			out = append(out, "Success. Downloaded item "+id)
		}
		return strings.Join(out, "\n"), nil
	}
	op, err := e.App.UpdateSubscriptions(nil, false)
	if err != nil || len(calls) != 1 || !slices.Equal(calls[0], []string{"8100", "8200"}) || len(op.Items) != 3 || !strings.HasPrefix(op.Items[1].Result, "skipped:") {
		t.Fatalf("groups %v, result %+v, error %v", calls, op, err)
	}
}

func TestUpdateWorkshopJunctionCacheAndRetiredSource(t *testing.T) {
	t.Parallel()
	for _, retired := range []bool{false, true} {
		t.Run(fmt.Sprint(retired), func(t *testing.T) {
			e := setup(t, false)
			root := filepath.Join(e.Root, "cache-library", "steamapps", "workshop", "content", "294100")
			target := filepath.Join(root, "8100")
			e.WriteMod(target, testkit.Sample("cache.mod", "Cache Mod"))
			if err := subscribe.Write(target, &subscribe.Record{Kind: "steamcmd", WorkshopID: "8100", Installed: e.Now().Format(time.RFC3339), InstalledAt: e.Now()}); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(e.Root, "library-alias")
			if err := source.CreateLink(alias, root); err != nil {
				t.Fatal(err)
			}
			added, err := e.App.FolderAdd(source.SteamCMD, alias)
			if err != nil {
				t.Fatal(err)
			}
			scanNow(t, e)
			ss, err := e.App.Subscriptions()
			if err != nil || len(ss) != 1 {
				t.Fatalf("subscription: %+v %v", ss, err)
			}
			mod, err := e.App.Store.Mod(ss[0].Mod.ID) // a separate copy to move
			if err != nil || mod == nil {
				t.Fatalf("mod: %v %v", mod, err)
			}
			mod.RealPath = filepath.Join(alias, "8100")
			cached := e.App.SubscriptionOf(mod)
			if cached == nil {
				t.Fatal("no alias provenance cached")
			}
			stamp, err := os.Stat(filepath.Join(target, subscribe.File))
			if err != nil {
				t.Fatal(err)
			}
			e.Advance(time.Hour)
			e.App.FindTool = func(string) (string, bool) {
				if retired {
					added.Source.Status = source.StatusRetired
					if err := e.App.Store.UpdateSource(added.Source); err != nil {
						t.Fatal(err)
					}
				}
				return filepath.Join(e.Root, "steamcmd.exe"), true
			}
			e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
				workshopScriptIDs(t, exe, args)
				return "Success. Downloaded item 8100", nil
			}
			if _, err := e.App.UpdateSubscriptions([]string{ss[0].Ref}, false); err != nil {
				t.Fatal(err)
			}
			newStamp, err := os.Stat(filepath.Join(target, subscribe.File))
			if err != nil || newStamp.Size() != stamp.Size() {
				t.Fatalf("cache fixture must keep record size: %v %v", newStamp, err)
			}
			if err := os.Chtimes(filepath.Join(target, subscribe.File), stamp.ModTime(), stamp.ModTime()); err != nil {
				t.Fatal(err)
			}
			if got := e.App.SubscriptionOf(mod); got == nil || got.Installed != e.Now().Format(time.RFC3339) || got.Installed == cached.Installed {
				t.Fatalf("stale alias cache: %+v", got)
			}
			folders, err := e.App.Folders()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, folder := range folders {
				if folder.ID == added.Source.ID {
					found = folder.Status != source.StatusRetired
				}
			}
			if !found {
				t.Fatal("existing source was not reactivated")
			}
		})
	}
}
