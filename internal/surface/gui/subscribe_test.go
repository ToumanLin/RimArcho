package gui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSubscriptionForm(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for subscription form behavior tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/subscribe.mjs", "testdata/subscriptions.mjs").CombinedOutput(); err != nil {
		t.Fatalf("subscription form: %v\n%s", err, out)
	}
}

func TestSubscriptionAPITasks(t *testing.T) {
	e, api, events := fixture(t)
	g := testkit.NewGitRepo(t, "")
	r := SubscriptionRequest{Kind: "git", URL: g.Bare, RefType: "branch", Ref: "main"}
	p, err := api.SubscribePreview(r)
	if err != nil || !p.Apply || !strings.Contains(strings.Join(p.Lines, " "), "nothing changed") {
		t.Fatalf("preview: %+v %v", p, err)
	}
	if _, err := os.Stat(e.App.SubscribedDir()); !os.IsNotExist(err) || len(api.tasks.List()) != 0 {
		t.Fatal("preview changed files or queued a task")
	}
	if _, err := api.Subscribe(r); err != nil {
		t.Fatal(err)
	}
	task := endedTask(t, api, view.TaskSubscribe)
	if task.State != app.TaskDone {
		t.Fatalf("install: %+v", task)
	}
	var installed app.Task
	for _, task := range api.tasks.List() {
		if task.Kind == view.TaskSubscribe {
			installed = task
		}
	}
	if installed.Target == "" {
		t.Fatal("completed install has no row to select")
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-events:
			if strings.Contains(event, `"ended"`) && strings.Contains(event, installed.ID) {
				if !strings.Contains(event, `"ref":"`+installed.Target+`"`) {
					t.Fatal(event)
				}
				goto selected
			}
		case <-deadline:
			t.Fatal("no selection event")
		}
	}
selected:
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 1 {
		t.Fatalf("subscriptions: %+v %v", ss, err)
	}
	sub := ss[0]
	if sub.Ref != installed.Target || sub.Record.Installed != g.First {
		t.Fatalf("installed: %+v", sub)
	}
	if p, err := api.Library("", []string{"source:git"}, ""); err != nil || len(p.Rows) != 1 || p.Rows[0].Insts[0].Source != "Git" {
		t.Fatalf("library: %+v %v", p, err)
	}
	next := g.Commit(t, "Defs/Example.xml", "<Defs/>")
	if _, err := api.LibraryDo(view.OpSubCheck, []string{sub.Ref}); err != nil {
		t.Fatal(err)
	}
	check := endedTask(t, api, view.OpSubCheck)
	waitSettingEnd(t, events, check.ID)
	if check.State != app.TaskDone {
		t.Fatalf("check: %+v", check)
	}
	p, err = api.LibraryPreview(view.OpSubUpdate, []string{sub.Ref})
	if err != nil || !p.Apply || !strings.Contains(strings.Join(p.Lines, " "), next) {
		t.Fatalf("update preview: %+v %v", p, err)
	}
	if r, _ := subscribe.Read(sub.Folder); r.Installed != g.First || !r.HasUpdate() {
		t.Fatalf("dry run wrote: %+v", r)
	}
	if p, err := api.Library("", []string{"state:source-update"}, ""); err != nil || len(p.Rows) != 1 {
		t.Fatalf("checked library: %+v %v", p, err)
	}
	if _, err := api.LibraryDo(view.OpSubUpdate, []string{sub.Ref}); err != nil {
		t.Fatal(err)
	}
	update := endedTask(t, api, view.OpSubUpdate)
	waitSettingEnd(t, events, update.ID)
	if update.State != app.TaskDone {
		t.Fatalf("update: %+v", update)
	}
	ss, err = e.App.Subscriptions()
	if err != nil || len(ss) != 1 || ss[0].Mod.ID != sub.Mod.ID || ss[0].Record.Installed != next {
		t.Fatalf("updated: %+v %v", ss, err)
	}
	e.MustRun("enable", sub.Ref)
	p, err = api.LibraryPreview(view.OpSubRemove, []string{sub.Ref})
	if err != nil || !p.Apply || !strings.Contains(strings.Join(p.Lines, " "), "default") {
		t.Fatalf("remove preview: %+v %v", p, err)
	}
	trashed := filepath.Join(t.TempDir(), "recycled")
	e.App.TrashFunc = func(path string) error { return os.Rename(path, trashed) }
	if _, err := api.LibraryDo(view.OpSubRemove, []string{sub.Ref}); err != nil {
		t.Fatal(err)
	}
	remove := endedTask(t, api, view.OpSubRemove)
	waitSettingEnd(t, events, remove.ID)
	if remove.State != app.TaskDone {
		t.Fatalf("remove: %+v", remove)
	}
	if _, err := os.Stat(filepath.Join(trashed, subscribe.File)); err != nil {
		t.Fatal(err)
	}
	if p, err := api.Library("", []string{"source:git"}, ""); err != nil || len(p.Rows) != 0 {
		t.Fatalf("removed library: %+v %v", p, err)
	}
}

func TestPageSubscriptionAPITasks(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	var version atomic.Int32
	version.Store(1)
	started, release := make(chan struct{}), make(chan struct{})
	var block atomic.Bool
	var once sync.Once
	defer once.Do(func() { close(release) })
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("unexpected page request: %s", r.URL)
		}
		if block.Load() {
			close(started)
			<-release
		}
		fmt.Fprintf(w, `<script type="application/ld+json">{"name":"Example Colony Tweaks","softwareVersion":"%d","dateModified":"2026-10-24T15:00:00+0000"}</script><a href="https://github.com/Example/dependency/releases">dependency</a><a href="https://gitgud.io/Example/colony/-/tree/main">mod</a><a href="?do=download">download</a>`, version.Load())
	}))
	t.Cleanup(srv.Close)
	e.App.HTTP = srv.Client()
	p, err := api.SubscriptionPage(srv.URL)
	if err != nil || p.Main != "https://gitgud.io/Example/colony" || len(p.Repos) != 2 || p.Name != "Example Colony Tweaks" {
		t.Fatalf("page: %+v %v", p, err)
	}
	files := filepath.Join(t.TempDir(), "downloaded")
	e.WriteMod(files, testkit.Sample("example.colony", "Example Colony Tweaks"))
	r := SubscriptionRequest{Kind: "page", URL: srv.URL, From: files}
	preview, err := api.SubscribePreview(r)
	if err != nil || !preview.Apply {
		t.Fatalf("page preview: %+v %v", preview, err)
	}
	if _, err := api.Subscribe(r); err != nil {
		t.Fatal(err)
	}
	install := endedTask(t, api, view.TaskSubscribe)
	waitSettingEnd(t, events, install.ID)
	if install.State != app.TaskDone {
		t.Fatalf("page install: %+v", install)
	}
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 1 || ss[0].Record.Kind != "page" {
		t.Fatalf("page sub: %+v %v", ss, err)
	}
	sub := ss[0]
	version.Store(2)
	block.Store(true)
	if _, err := api.LibraryDo(view.OpSubCheck, []string{sub.Ref}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("check never started")
	}
	read := make(chan struct{})
	go func() { api.Setting("ui.gui.library.side"); close(read) }()
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatal("page check held the GUI lock")
	}
	block.Store(false)
	once.Do(func() { close(release) })
	check := endedTask(t, api, view.OpSubCheck)
	waitSettingEnd(t, events, check.ID)
	if check.State != app.TaskDone {
		t.Fatalf("page check: %+v", check)
	}
	// Bulk updates skip pages even when only one selected item has an update action.
	preview, err = api.LibraryPreview(view.OpSubUpdate, []string{sub.Ref})
	if err != nil || !strings.Contains(strings.Join(preview.Lines, " "), "skipped") {
		t.Fatalf("page skip preview: %+v %v", preview, err)
	}
	note, err := api.LibraryDo(view.OpSubUpdate, []string{sub.Ref})
	if err != nil || !strings.Contains(note, "skipped") {
		t.Fatalf("page skip: %s %v", note, err)
	}
	e.WriteFile(filepath.Join(files, "Defs", "Example.xml"), "<Defs/>")
	preview, err = api.SubscriptionFromPreview(sub.Ref, files)
	if err != nil || !preview.Apply || !strings.Contains(strings.Join(preview.Lines, " "), "2 @") {
		t.Fatalf("replacement preview: %+v %v", preview, err)
	}
	e.App.TrashFunc = func(path string) error { return os.Rename(path, filepath.Join(t.TempDir(), "recycled")) }
	if _, err := api.SubscriptionFrom(sub.Ref, files); err != nil {
		t.Fatal(err)
	}
	update := endedTask(t, api, view.TaskSubscriptionFrom)
	waitSettingEnd(t, events, update.ID)
	if update.State != app.TaskDone {
		t.Fatalf("page update: %+v", update)
	}
	ss, err = e.App.Subscriptions()
	if err != nil || len(ss) != 1 || ss[0].Mod.ID != sub.Mod.ID || !strings.HasPrefix(ss[0].Record.Installed, "2 @") || ss[0].Record.HasUpdate() {
		t.Fatalf("page updated: %+v %v", ss, err)
	}
}

func TestWorkshopSubscriptionTaskMessage(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64_Data", "Plugins", "x86_64", "steam_api64.dll"), "stand-in")
	e.App.SteamWorker = func(_ string, args []string) (string, error) {
		if strings.Join(args, " ") != "subscribe 1234" {
			t.Errorf("Steam args: %v", args)
		}
		return steamworks.ItemLine(1234, "ok", steamworks.ItemSubscribed|steamworks.ItemDownloadPending) + "\n" + steamworks.DoneLine(steamworks.Subscribe, 1), nil
	}
	if _, err := api.Subscribe(SubscriptionRequest{Kind: "workshop", URL: "1234"}); err != nil {
		t.Fatal(err)
	}
	task := endedTask(t, api, view.TaskSubscribe)
	waitSettingEnd(t, events, task.ID)
	if task.State != app.TaskDone || !strings.Contains(task.Say, "next scan") || len(task.Items) != 1 || task.Items[0].State != app.ItemOK {
		t.Fatalf("Steam result: %+v", task)
	}
}
