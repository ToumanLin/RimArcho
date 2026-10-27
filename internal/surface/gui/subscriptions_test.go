package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSubscriptionSourceTaskAndRunningGuard(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	if _, err := api.CheckSubscriptionSource("invalid"); err == nil || len(api.tasks.List()) != 0 {
		t.Fatal("invalid source queued")
	}
	p, err := api.SubscriptionsPage(false, "")
	if err != nil || len(p.Sources) != 5 || p.History == nil {
		t.Fatalf("page: %+v %v", p, err)
	}
	if _, err := api.CheckSubscriptionSource("git"); err != nil {
		t.Fatal(err)
	}
	task := endedTask(t, api, view.TaskSourceCheck)
	if task.State != app.TaskDone {
		t.Fatalf("check: %+v", task)
	}
	if e.App.SubscriptionSourceCheck("git").At.IsZero() {
		t.Fatal("check did not persist its result")
	}
	if got := task.Target; got != view.PageChanges {
		t.Fatalf("task o: %q", got)
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
	if _, err := api.WorkshopUpdatePreview("ws:1001"); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("running preview: %v", err)
	}
	if _, err := api.WorkshopUpdate("ws:1001"); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("running apply: %v", err)
	}
}

func TestSubscriptionInventoryReuseAndInvalidation(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ix.ResolveOne("ws:1001")
	if err != nil {
		t.Fatal(err)
	}
	read := func(library bool, of string) *view.SubscriptionsPage {
		t.Helper()
		p, err := api.SubscriptionsPage(library, of)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	read(false, "")
	first := api.subRead.lib
	if api.subRead.a == e.App || api.subRead.a.Store.DB == e.App.Store.DB || first.St.Ix.ByID[m.ID] == m {
		t.Fatal("subscription reader shares mutable application/index state")
	}
	for _, scope := range []struct {
		library bool
		of      string
	}{{true, ""}, {false, m.ID}, {false, ""}} {
		p := read(scope.library, scope.of)
		if api.subRead.lib != first {
			t.Fatal("scope switch rebuilt unchanged inventory")
		}
		st, err := e.App.Loadout()
		if err != nil {
			t.Fatal(err)
		}
		o := modchanges.Opts{Library: scope.library}
		if scope.of != "" {
			o.Of = view.LineageIDs(st.Ix, scope.of)
			o.OfName = st.Ix.ByID[scope.of].DisplayName()
		}
		want, err := view.BuildSubscriptions(e.App, st, o)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(p, want) {
			t.Fatalf("cached view differs for %+v", scope)
		}
	}
	changed := func(what string, mutate func()) {
		t.Helper()
		old := api.subRead.lib
		mutate()
		read(false, "")
		if api.subRead.lib == old {
			t.Fatalf("%s did not invalidate inventory", what)
		}
		current := api.subRead.lib
		read(false, "")
		if api.subRead.lib != current {
			t.Fatalf("%s cache did not settle", what)
		}
	}
	changed("tags", func() {
		_, err := e.App.SetTags([]string{"ws:1001"}, []string{"fav"}, false)
		if err != nil {
			t.Fatal(err)
		}
	})
	if !reflect.DeepEqual(api.subRead.lib.St.Ix.ByID[m.ID].Tags, []string{"fav"}) {
		t.Fatal("stale tags")
	}
	changed("note", func() {
		if err := e.App.SetNote("ws:1001", "remember"); err != nil {
			t.Fatal(err)
		}
	})
	if api.subRead.lib.St.Ix.ByID[m.ID].Note != "remember" {
		t.Fatal("stale note")
	}
	changed("alias", func() {
		if err := e.App.SetAlias("ws:1001", "Renamed"); err != nil {
			t.Fatal(err)
		}
	})
	if api.subRead.lib.St.Ix.ByID[m.ID].DisplayName() != "Renamed" {
		t.Fatal("stale alias")
	}
	changed("external commit", func() {
		r, err := e.App.ReadSession()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		if err := r.Store.SetMeta("test:external", "changed"); err != nil {
			t.Fatal(err)
		}
	})
	changed("task without database write", func() { api.tasks.Ended(app.Task{Lane: app.LaneSelf, Kind: view.TaskScan}) })
	changed("ACF without scan", func() { e.Subscribe("99999") })
	ghost := false
	for _, l := range api.subRead.lib.Lineages {
		for _, i := range l.Instances {
			if i.Ref == "ws:99999" && i.Missing() {
				ghost = true
			}
		}
	}
	if !ghost {
		t.Fatal("new Steam subscription missing")
	}
	oldSnap := api.subRead.lib.St.Ix.ByID[m.ID].SnapID
	changed("scan snapshot", func() {
		mod := testkit.Sample("tester.a", "Scanned Alpha")
		mod.Files["Defs/New.xml"] = "<Defs/>"
		e.AddWorkshop("1001", mod)
		e.MustRun("scan")
	})
	if api.subRead.lib.St.Ix.ByID[m.ID].SnapID == oldSnap {
		t.Fatal("stale snapshot")
	}
	plan, err := e.App.NewPlan("Other list", "")
	if err != nil {
		t.Fatal(err)
	}
	read(false, "")
	changed("open plan without DB write", func() { e.App.OpenPlan = plan.Plan.ID })
	if api.subRead.lib.St.Plan.ID != plan.Plan.ID {
		t.Fatal("stale current list")
	}
	changed("game version", func() { e.App.GameVersion = "1.7" })
	e.MustRun("env", "add", "other", "--game", e.OtherGame("Other", "1.6.4633 rev1261"), "--saves", "own")
	read(false, "")
	changed("environment option", func() { e.App.EnvName = "other" })
	if api.subRead.lib.Env.Name != "other" {
		t.Fatal("stale environment")
	}
}

func TestAddSubscriptionInfoSkipsInventoryAndUninstall(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	p, err := api.AddSubscriptionInfo()
	if err != nil || p == nil || len(p.Sources) != 2 {
		t.Fatalf("info: %+v %v", p, err)
	}
	if api.subRead.lib != nil {
		t.Fatal("add form built full inventory")
	}
	e.App.Uninstalled = true
	if _, err := api.AddSubscriptionInfo(); err == nil {
		t.Fatal("uninstalled add allowed")
	}
	if _, err := api.SubscriptionsPage(false, ""); err == nil {
		t.Fatal("uninstalled page allowed")
	}
}

func TestSubscriptionReadDoesNotBlockEditsAndRechecksAfterOvertake(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	b := api.Board()
	id := b.Cols[view.Active][1].ID
	started, release := pauseInventory(t, api)
	done := make(chan error, 1)
	go func() { _, err := api.SubscriptionsPage(false, ""); done <- err }()
	awaitInventory(t, started)
	toggleResponds(t, api, id)
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	old := api.subRead.lib
	if _, err := api.SubscriptionsPage(false, ""); err != nil {
		t.Fatal(err)
	}
	if api.subRead.lib == old {
		t.Fatal("overtaken inventory reused after edit")
	}
	for _, l := range api.subRead.lib.Lineages {
		for _, i := range l.Instances {
			if i.Mod != nil && i.Mod.ID == id && i.InPlan {
				t.Fatal("stale plan membership")
			}
		}
	}
}

func TestSubscriptionReaderSeesSourceRecordChanges(t *testing.T) {
	e, api, _ := fixture(t)
	g := testkit.NewGitRepo(t, "")
	op, err := e.App.SubscribeGit(g.Bare, "branch", "main", false)
	if err != nil {
		t.Fatal(err)
	}
	folder := op.Items[0].Folder
	if _, err := api.SubscriptionsPage(true, ""); err != nil {
		t.Fatal(err)
	}
	old := api.subRead.lib
	r, err := subscribe.Read(folder)
	if err != nil {
		t.Fatal(err)
	}
	r.Latest = "new-upstream"
	r.CheckedAt = e.Now().Add(time.Hour)
	if err := subscribe.Write(folder, r); err != nil {
		t.Fatal(err)
	}
	// Force a distinct timestamp on filesystems with coarse clocks.
	stamp := e.Now().Add(2 * time.Hour)
	if err := os.Chtimes(filepath.Join(folder, subscribe.File), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	p, err := api.SubscriptionsPage(true, "")
	if err != nil {
		t.Fatal(err)
	}
	if api.subRead.lib == old || len(p.Pending) != 1 || p.Pending[0].Available != "new-ups" {
		t.Fatalf("stale source: %+v", p.Pending)
	}
	info, err := api.AddSubscriptionInfo()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(info.Recent, p.Recent) {
		t.Fatalf("light recent mismatch: %+v / %+v", info.Recent, p.Recent)
	}
}

func TestSubscriptionReaderImportsExternalListBeforeReusingInventory(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	if _, err := api.SubscriptionsPage(false, ""); err != nil {
		t.Fatal(err)
	}
	old := api.subRead.lib
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.b")
	if _, err := api.SubscriptionsPage(false, ""); err != nil {
		t.Fatal(err)
	}
	if api.subRead.lib == old {
		t.Fatal("external list imported but inventory reused")
	}
	for _, l := range api.subRead.lib.Lineages {
		for _, i := range l.Instances {
			if i.Mod == nil {
				continue
			}
			if i.Mod.PackageID == "tester.a" && i.InPlan || i.Mod.PackageID == "tester.b" && !i.InPlan {
				t.Fatalf("stale external membership: %+v", i)
			}
		}
	}
}

func TestSubscriptionPackUsageTracksDiskChanges(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	e.SetTimeUpdated("1001", e.Now().Unix())
	e.MustRun("scan")
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ix.ResolveOne("ws:1001")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(app.SourceCheck{At: e.Now(), Latest: map[string]int64{m.ID: e.Now().Add(time.Hour).Unix()}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.SetMeta("subscriptions:check:main:workshop", string(b)); err != nil {
		t.Fatal(err)
	}
	read := func(want string) {
		t.Helper()
		p, err := api.SubscriptionsPage(true, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Pending) != 1 || p.Pending[0].ID != m.ID {
			t.Fatalf("pending: %+v", p.Pending)
		}
		var packs []string
		for _, u := range api.subRead.lib.Lineages {
			for _, i := range u.Instances {
				if i.Mod == nil || i.Mod.ID != m.ID {
					continue
				}
				for _, use := range i.UsedIn {
					if use.Kind == "pack" {
						packs = append(packs, use.Name)
					}
				}
			}
		}
		if strings.Join(packs, ",") != want {
			t.Fatalf("pack uses: %v, want %q", packs, want)
		}
		if want != "" {
			found := false
			for _, name := range p.Pending[0].UsedIn {
				if name == want {
					found = true
				}
			}
			if !found {
				t.Fatalf("pack absent from pending usage: %+v", p.Pending[0])
			}
		} else {
			for _, name := range p.Pending[0].UsedIn {
				if name == "Pack A" || name == "Pack B" {
					t.Fatalf("stale pending usage: %+v", p.Pending[0])
				}
			}
		}
	}
	read("")
	changeKey, revision := e.App.Store.ChangeKey(), api.subRevision.Load()
	manifest := filepath.Join(e.App.PacksDir(), "fixture", "pack.json")
	write := func(name string) {
		t.Helper()
		b, err := json.Marshal(app.Pack{Format: app.PackFormat, Name: name, Env: "main", Entries: []app.PackEntry{{Instance: m.ID, PackageID: m.PackageID, Bytes: 1}}})
		if err != nil {
			t.Fatal(err)
		}
		e.WriteFile(manifest, string(b))
	}
	old := api.subRead.lib
	write("Pack A")
	read("Pack A")
	if api.subRead.lib == old {
		t.Fatal("external addition did not invalidate inventory")
	}
	old = api.subRead.lib
	root, err := os.Stat(e.App.PacksDir())
	if err != nil {
		t.Fatal(err)
	}
	write("Pack B") // Same-size edit, with no packs directory change.
	stamp := e.Now().Add(2 * time.Hour)
	if err := os.Chtimes(manifest, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(e.App.PacksDir(), root.ModTime(), root.ModTime()); err != nil {
		t.Fatal(err)
	}
	read("Pack B")
	if api.subRead.lib == old {
		t.Fatal("manifest edit did not invalidate inventory")
	}
	old = api.subRead.lib
	e.Remove(filepath.Dir(manifest))
	// Enumeration catches a removal even if a filesystem preserves root mtime.
	if err := os.Chtimes(e.App.PacksDir(), root.ModTime(), root.ModTime()); err != nil {
		t.Fatal(err)
	}
	read("")
	if api.subRead.lib == old {
		t.Fatal("external removal did not invalidate inventory")
	}
	old = api.subRead.lib
	read("")
	if api.subRead.lib != old {
		t.Fatal("unchanged packs did not reuse inventory")
	}
	if e.App.Store.ChangeKey() != changeKey || api.subRevision.Load() != revision {
		t.Fatal("disk-change test changed database or revision")
	}
}

func TestSubscriptionPackEntrypointsInvalidate(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	e.App.TrashFunc = os.RemoveAll
	b, err := json.Marshal(app.Pack{Format: app.PackFormat, Name: "fixture", Env: "main", Entries: []app.PackEntry{}})
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(e.App.PacksDir(), "fixture", "pack.json"), string(b))
	archive := filepath.Join(e.Root, "fixture.archopack")
	if _, err := e.App.ExportPack("fixture", archive, false); err != nil {
		t.Fatal(err)
	}
	check := func(name string, mutate func() (string, error)) {
		t.Helper()
		if _, err := api.SubscriptionsPage(true, ""); err != nil {
			t.Fatal(err)
		}
		old, revision := api.subRead.lib, api.subRevision.Load()
		if _, err := mutate(); err != nil {
			t.Fatal(err)
		}
		if api.subRevision.Load() <= revision {
			t.Fatalf("%s did not advance revision", name)
		}
		if _, err := api.SubscriptionsPage(true, ""); err != nil {
			t.Fatal(err)
		}
		if api.subRead.lib == old {
			t.Fatalf("%s reused inventory", name)
		}
	}
	check("remove", func() (string, error) { return api.PackRemove("fixture") })
	check("import", func() (string, error) { return api.PackImport(archive) })
	b, err = json.Marshal(app.BuildProgress{Name: "cut", Step: app.StepCopy, Beat: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(e.App.PacksDir(), "cut"+app.TmpSuffix, app.BuildFile), string(b))
	check("unfinished removal", func() (string, error) { return api.PackRemoveUnfinished("cut") })
}

func TestSubscriptionReaderReleasesInventory(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	read := func() {
		t.Helper()
		if _, err := api.SubscriptionsPage(true, ""); err != nil {
			t.Fatal(err)
		}
		if api.subRead.a == nil || api.subRead.lib == nil || api.subRead.idle == nil {
			t.Fatal("reader not retained")
		}
	}
	released := func() {
		t.Helper()
		api.subRead.mu.Lock()
		defer api.subRead.mu.Unlock()
		if api.subRead.a != nil || api.subRead.lib != nil || api.subRead.idle != nil || api.subRead.key != "" {
			t.Fatal("reader still retained")
		}
	}
	read()
	old := api.subRead.lib
	api.expireSubscriptions() // A queued old callback must preserve a recently used reader.
	if api.subRead.lib != old {
		t.Fatal("recent reader expired")
	}
	api.subRead.mu.Lock()
	api.subRead.used = time.Now().Add(-subscriptionIdle)
	api.subRead.mu.Unlock()
	api.expireSubscriptions()
	released()
	read() // Reopen after expiry.
	e.MustRun("env", "add", "other", "--game", e.OtherGame("Other", "1.6.4633 rev1261"), "--saves", "own")
	if !api.UseEnv(1, "other") {
		t.Fatal("switch rejected")
	}
	if r := environmentResult(t, events, 1); r.Error != "" || r.Stale {
		t.Fatalf("switch: %+v", r)
	}
	released() // No subscription request in the new environment is needed to release it.
	read()
	if api.subRead.lib.Env.Name != "other" {
		t.Fatal("wrong rebuilt environment")
	}
	api.Close()
	released()
}
