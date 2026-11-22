package app_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/corpus"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/updatechannel"
)

// writeCorpus puts a corpus bundle with a 1.6 section of week into a Monitor Mod folder.
func writeCorpus(t *testing.T, e *testkit.Env, modDir, week string) {
	t.Helper()
	b := &corpus.Bundle{Format: corpus.Format, Data: week, Sections: map[string]*corpus.Section{
		"1.6": {Data: week, Logs: 1234, Patterns: []byte(`{"version":"w","game":"1.6","logs":1234,"generic":[{"id":"g:week.` + week + `","match":"x","logs":1,"sessions":1,"clusters":1}]}`)},
	}}
	enc, err := corpus.Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(modDir, corpus.Dir, corpus.File), string(enc))
}

// 19 §6, 05 §4.6: the newest corpus section among the embedded bundle (A), RimArcho's Monitor
// folders (B) and a Workshop Monitor (C) is the data in use; a data-only update of the same
// Monitor version refreshes its folder in place instead of being "same version, different
// content".
func TestMonitorCorpusData(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	if ch := e.App.Corpus(); ch.Origin != corpus.Embedded || ch.Data != corpus.EmbeddedData {
		t.Fatalf("before any Monitor: %+v", ch.Summarize())
	}
	if out := e.MustRun("status"); !strings.Contains(out, "corpus: W43, 1.6; built in") {
		t.Fatalf("status without the corpus line:\n%s", out)
	}
	bundled := monitorBundle(t, e, "0.12.0")
	writeCorpus(t, e, bundled, "2026-W47")
	e.App.MonitorBundle = bundled
	e.MustRun("monitor", "install")
	target := filepath.Join(e.App.MonitorDir(), "ArchoMonitor-0.12.0")
	ch := e.App.Corpus()
	if ch.Origin != corpus.Monitor || ch.Data != "2026-W47" || ch.Label != "ArchoMonitor-0.12.0" {
		t.Fatalf("B not chosen: %+v", ch.Summarize())
	}
	if e.App.Patterns().Get("g:week.2026-W47") == nil {
		t.Fatal("the environment does not use the chosen section")
	}
	// A data-only update: same modVersion and DLL, newer bundle.
	writeCorpus(t, e, bundled, "2026-W48")
	out := e.MustRun("monitor", "install")
	if strings.Contains(out, "never replaced") || strings.Contains(out, "different") {
		t.Fatalf("a data-only update was treated as changed content:\n%s", out)
	}
	if ents, _ := os.ReadDir(e.App.MonitorDir()); len(ents) != 1 {
		t.Fatalf("a data-only update made a new folder: %v", ents)
	}
	if ch := e.App.Corpus(); ch.Data != "2026-W48" {
		t.Fatalf("corpus not refreshed in place: %+v", ch.Summarize())
	}
	if _, err := os.Stat(filepath.Join(target, corpus.Dir, corpus.File)); err != nil {
		t.Fatal(err)
	}
	// C: a Workshop Monitor with newer data wins.
	ws := e.AddWorkshop("9999", testkit.Mod{PackageID: app.MonitorPackage, Name: monName})
	about, _ := os.ReadFile(filepath.Join(bundled, "About", "About.xml"))
	e.WriteFile(filepath.Join(ws, "About", "About.xml"), string(about))
	writeCorpus(t, e, ws, "2026-W49")
	scanNow(t, e)
	if ch := e.App.Corpus(); ch.Origin != corpus.Workshop || ch.Data != "2026-W49" {
		t.Fatalf("C not chosen: %+v", ch.Summarize())
	}
	if out := e.MustRun("status"); !strings.Contains(out, "corpus: W49, 1.6; Workshop Archo Monitor, 1234 logs") {
		t.Fatalf("status:\n%s", out)
	}
}

// 19 §6 "anonymous fetch": without a Workshop Monitor, the automatic check downloads the item
// with +login anonymous into a scratch folder and installs it as a Monitor folder; it never
// runs with automatic checks off, and not again until the item changes.
func TestMonitorAnonymousFetch(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	t.Setenv(updatechannel.MonitorItemEnv, "12345")
	updated := 1_790_000_000
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[{"publishedfileid":"12345","result":1,"time_updated":%d}]}}`, updated)
	}))
	defer srv.Close()
	e.App.SteamAPI, e.App.HTTP = srv.URL, srv.Client()
	dir := e.App.SteamCMDDir()
	e.WriteFile(filepath.Join(dir, deps.SteamCMDExecutable()), "fake")
	e.App.SteamCMDNoPath = true
	var runs [][]string
	e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
		runs = append(runs, args)
		base := args[slices.Index(args, "+force_install_dir")+1]
		item := filepath.Join(base, "steamapps", "workshop", "content", "294100", "12345")
		src := monitorBundle(t, e, "0.12.0")
		writeCorpus(t, e, src, "2026-W47")
		if err := os.CopyFS(item, os.DirFS(src)); err != nil {
			t.Error(err)
		}
		return "Success. Downloaded item 12345", nil
	}
	if err := e.App.Store.SetMeta("cfg:updates.auto", "off"); err != nil {
		t.Fatal(err)
	}
	if f, err := e.App.FetchMonitorAnonymous(context.Background()); err != nil || f.Skipped != "automatic checks are off" || len(runs) != 0 {
		t.Fatalf("ran with automatic checks off: %+v %v", f, err)
	}
	e.App.Store.SetMeta("cfg:updates.auto", "")
	f, err := e.App.FetchMonitorAnonymous(context.Background())
	if err != nil || !f.Downloaded || f.Version != "0.12.0" || !f.Copied || len(runs) != 1 {
		t.Fatalf("fetch: %+v %v (%d runs)", f, err, len(runs))
	}
	if i := slices.Index(runs[0], "+login"); i < 0 || runs[0][i+1] != "anonymous" {
		t.Fatalf("not anonymous: %v", runs[0])
	}
	if ch := e.App.Corpus(); ch.Origin != corpus.Monitor || ch.Data != "2026-W47" {
		t.Fatalf("downloaded data not in use: %+v", ch.Summarize())
	}
	if _, err := os.Stat(filepath.Join(e.App.LocalDir(), "monitor-download")); !os.IsNotExist(err) {
		t.Fatalf("scratch folder left behind: %v", err)
	}
	if f, _ := e.App.FetchMonitorAnonymous(context.Background()); f.Skipped != "up to date" || len(runs) != 1 {
		t.Fatalf("second check: %+v", f)
	}
	// A Workshop-subscribed Monitor makes the anonymous copy unnecessary.
	updated++
	ws := e.AddWorkshop("9999", testkit.Mod{PackageID: app.MonitorPackage, Name: monName})
	about, _ := os.ReadFile(filepath.Join(monitorBundle(t, e, "0.12.0"), "About", "About.xml"))
	e.WriteFile(filepath.Join(ws, "About", "About.xml"), string(about))
	scanNow(t, e)
	if f, _ := e.App.FetchMonitorAnonymous(context.Background()); !strings.Contains(f.Skipped, "Workshop") || len(runs) != 1 {
		t.Fatalf("with a Workshop Monitor: %+v", f)
	}
}

// 05 §4.6: corpus data belongs to an environment, not the process. Two Apps of different game
// versions working at once each read their own version's patterns and prerequisites.
func TestCorpusPerEnvironment(t *testing.T) {
	t.Parallel()
	home := func(ver string) *testkit.Env {
		e := testkit.New(t)
		e.WriteFile(filepath.Join(e.Game, "Version.txt"), ver)
		e.WriteModsConfig(ver, "ludeon.rimworld")
		e.AddEnv(false)
		e.MustRun("scan")
		return e
	}
	e16, e17 := home("1.6.4633 rev1261"), home("1.7.5000 rev1")
	want := func(e *testkit.Env) *corpus.Active {
		env, _ := e.App.Env()
		return e.App.CorpusData(env)
	}
	w16, w17 := want(e16), want(e17)
	if e16.App.Corpus().Game != "1.6" || e17.App.Corpus().Game != "1.7" {
		t.Fatalf("choices: %+v / %+v", e16.App.Corpus().Summarize(), e17.App.Corpus().Summarize())
	}
	errs := make(chan string, 2)
	check := func(e *testkit.Env, w *corpus.Active) {
		for range 30 {
			ix, err := e.App.Index()
			if err != nil {
				errs <- err.Error()
				return
			}
			if ix.Patterns() != w.Patterns() || ix.Known() != w.Known() {
				errs <- "an environment read another's corpus data"
				return
			}
		}
		errs <- ""
	}
	go check(e16, w16)
	go check(e17, w17)
	for range 2 {
		if m := <-errs; m != "" {
			t.Fatal(m)
		}
	}
}
