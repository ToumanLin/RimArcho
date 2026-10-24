package app_test

import (
	"archive/zip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func pageZip(t *testing.T, wrapper, content string, unsafe bool) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mod.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	entries := map[string]string{wrapper + "About/About.xml": `<ModMetaData><name>Example Colony Tweaks</name><packageId>example.colony.tweaks</packageId></ModMetaData>`, wrapper + "Defs/Test.xml": content}
	if unsafe {
		entries["../outside.txt"] = "escape"
	}
	for name, text := range entries {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSubscribePage(t *testing.T) {
	e := setup(t, false)
	version := "1.0"
	date := "2026-10-23T15:16:59Z"
	links := `<a href="https://gitgud.io/Example/colony/-/tree/main">repo</a>`
	status := http.StatusOK
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.WriteHeader(status)
		fmt.Fprintf(w, `<script type="application/ld+json">{"@graph":[{"@type":"SoftwareApplication","name":"Example Colony Tweaks","softwareVersion":%q,"dateModified":%q}]}</script>%s<a href="?do=download">download</a>`, version, date, links)
	}))
	defer srv.Close()
	e.App.HTTP = srv.Client()
	op, err := e.App.SubscribePage(srv.URL, false, "", false)
	if err != nil || !op.DryRun || calls != 1 || !strings.Contains(strings.Join(op.Notes, " "), "--from") {
		t.Fatalf("inspect: %+v %v", op, err)
	}
	if _, err := os.Stat(e.App.SubscribedDir()); !os.IsNotExist(err) {
		t.Fatal("inspect wrote files")
	}
	g := testkit.NewGitRepo(t, "")
	app.ResolvePageRepoForTest(e.App, func(repo string) string {
		if repo != "https://gitgud.io/Example/colony" {
			t.Fatal(repo)
		}
		return g.Bare
	})
	op, err = e.App.SubscribePage(srv.URL, true, "", false)
	if err != nil {
		t.Fatal(err)
	}
	r, err := subscribe.Read(op.Items[0].Folder)
	if err != nil || r.Kind != "git" || r.URL != srv.URL || r.Repo != g.Bare {
		t.Fatalf("Git record: %+v %v", r, err)
	}
	if _, err := e.App.SubscribeGit(g.Bare, "branch", "", false); err == nil {
		t.Fatal("page Git duplicate accepted")
	}
	second := g.Commit(t, "Defs/Git.xml", "<Defs/>")
	if _, err := e.App.CheckSubscriptions([]string{op.Items[0].Ref}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.UpdateSubscriptions([]string{op.Items[0].Ref}, false); err != nil {
		t.Fatal(err)
	}
	if gitHead(t, op.Items[0].Folder) != second {
		t.Fatal("Git update used page URL")
	}
	links += `<a href="https://github.com/Example/framework/releases">another</a>`
	if _, err := e.App.SubscribePage(srv.URL, true, "", false); err == nil || !strings.Contains(err.Error(), "https://github.com/Example/framework") {
		t.Fatalf("multiple repos: %v", err)
	}
	links = ""
	first := pageZip(t, "Colony/", "<Defs><ThingDef><defName>First</defName></ThingDef></Defs>", false)
	if _, err := e.App.SubscribePage(srv.URL+"/release", false, first, true); err != nil {
		t.Fatal(err)
	}
	before, _ := e.App.Subscriptions()
	if len(before) != 1 {
		t.Fatal("dry run installed page")
	}
	op, err = e.App.SubscribePage(srv.URL+"/release", false, first, false)
	if err != nil {
		t.Fatal(err)
	}
	it := op.Items[0]
	if it.ID == "" || !strings.Contains(it.Folder, "Example Colony Tweaks") {
		t.Fatalf("install: %+v", it)
	}
	if _, err := e.App.SubscribePage(srv.URL+"/release", false, first, false); err == nil {
		t.Fatal("duplicate page accepted")
	}
	version, date = "1.1", "2026-10-24T15:16:59Z"
	e.Advance(time.Hour)
	check, err := e.App.CheckSubscriptions([]string{it.Ref})
	if err != nil || check.Items[0].Result != "update available" {
		t.Fatalf("check: %+v %v", check, err)
	}
	r, _ = subscribe.Read(it.Folder)
	if !r.HasUpdate() {
		t.Fatal("page update not recorded")
	}
	if _, err := e.App.UpdateSubscriptions([]string{it.Ref}, false); err == nil || !strings.Contains(err.Error(), "--from") {
		t.Fatalf("page update without archive: %v", err)
	}
	bulk, err := e.App.UpdateSubscriptions(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	skipped := false
	for _, item := range bulk.Items {
		if item.ID == it.ID && strings.Contains(item.Result, "skipped") {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("page not skipped: %+v", bulk)
	}
	newZip := pageZip(t, "", "<Defs><ThingDef><defName>Second</defName></ThingDef></Defs>", false)
	var trashed []string
	e.App.TrashFunc = func(path string) error {
		trashed = append(trashed, path)
		return os.Rename(path, filepath.Join(e.Root, "old-page"))
	}
	if _, err := e.App.UpdateSubscriptionFrom(it.Ref, newZip, true); err != nil || len(trashed) != 0 {
		t.Fatalf("preview: %v %v", err, trashed)
	}
	bad := pageZip(t, "", "<Defs/>", true)
	if _, err := e.App.UpdateSubscriptionFrom(it.Ref, bad, false); err == nil || len(trashed) != 0 {
		t.Fatalf("zip slip: %v %v", err, trashed)
	}
	op, err = e.App.UpdateSubscriptionFrom(it.Ref, newZip, false)
	if err != nil || op.Items[0].ID != it.ID || len(trashed) != 1 {
		t.Fatalf("update: %+v %v trash %v", op, err, trashed)
	}
	r, _ = subscribe.Read(it.Folder)
	if r.Installed != "1.1 @ 2026-10-24T15:16:59Z" || r.HasUpdate() {
		t.Fatalf("updated record: %+v", r)
	}
	status = http.StatusForbidden
	if _, err := e.App.CheckSubscriptions([]string{it.Ref}); err != nil {
		t.Fatalf("page failure should be recorded: %v", err)
	}
	r, _ = subscribe.Read(it.Folder)
	if r.CheckError == "" || r.Latest != "1.1 @ 2026-10-24T15:16:59Z" {
		t.Fatalf("failed check: %+v", r)
	}
}
