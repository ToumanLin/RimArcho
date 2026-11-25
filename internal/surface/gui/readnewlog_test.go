//go:build windows || cgo

package gui_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/surface/gui"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// After a game, the window reads the new Player.log by itself (16 §2.1): Can it run's badge
// counts the run without a visit, the log is recorded as read, and a second call reads
// nothing. Until 2026-10-30 the sidebar said "analyse this run's log" and waited for a click.
func TestReadNewLogCountsTheRun(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.alpha", Name: "Alpha Props", Files: map[string]string{"Defs/Test.xml": "<Defs><ThingDef><defName>AlphaDef</defName></ThingDef></Defs>"}})
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.alpha")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	// the window's options (gui.Run): logs are read only from the cache until a page asks
	e.App.LazyLog, e.App.LogCacheOnly = true, true
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	badge := func() int {
		for _, g := range api.Shell().Nav {
			for _, it := range g.Items {
				if it.ID == view.PageNeeds {
					return it.Badge
				}
			}
		}
		t.Fatal("no Can it run in the sidebar")
		return 0
	}
	if api.ReadNewLog() {
		t.Fatal("read a log before there was one")
	}
	before := badge()
	e.RecordLaunch(e.Now().Add(-time.Minute))
	e.WriteFile(filepath.Join(e.UserData, "Player.log"), "RimWorld 1.6.4633 rev1\n\n"+
		strings.Repeat("Could not load Texture2D at 'Things/Alpha' for def 'AlphaDef'\n\n", 3))
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if _, newer := e.App.NewLog(st); !newer {
		t.Fatal("the fixture's log is not new")
	}
	if badge() != before {
		t.Fatalf("the cache-only sidebar counted an unread log: %d", badge())
	}
	if !api.ReadNewLog() {
		t.Fatal("did not read the new log")
	}
	if got := badge(); got != before+1 {
		t.Fatalf("badge %d after reading, want %d", got, before+1)
	}
	if _, newer := e.App.NewLog(st); newer {
		t.Fatal("the log is still new after reading it")
	}
	if api.ReadNewLog() {
		t.Fatal("read the same log twice")
	}
}

// A read log's stored report is keyed to the analysis and the Mods installed. When a later
// analysis or a Mod change makes it miss, Launch reads the log again instead of saying
// "not checked yet" until Can it run is opened (VM 2026-11-22: after an update and a check
// the last game line stayed "not checked yet", Can it run said no errors).
func TestHomeReadsALogWhoseReportNoLongerMatches(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.alpha", Name: "Alpha Props", Files: map[string]string{"Defs/Test.xml": "<Defs><ThingDef><defName>AlphaDef</defName></ThingDef></Defs>"}})
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.alpha")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.App.LazyLog, e.App.LogCacheOnly = true, true
	// Keep the Mod-file check's publication separate from the log refresh.
	e.App.AnalyzeRunner = func(*app.App, func(string)) error { return errors.New("fixture") }
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	e.RecordLaunch(e.Now().Add(-time.Minute))
	e.WriteFile(filepath.Join(e.UserData, "Player.log"), "RimWorld 1.6.4633 rev1\n\n"+
		strings.Repeat("Could not load Texture2D at 'Things/Alpha' for def 'AlphaDef'\n\n", 3))
	if !api.ReadNewLog() {
		t.Fatal("did not read the new log")
	}
	if h := api.Home(); h.LastRun == "" || h.LastRunPending {
		t.Fatalf("after reading: last run %q, pending %v", h.LastRun, h.LastRunPending)
	}
	// another Mod comes in (Steam downloads one): the stored report no longer matches
	e.AddWorkshop("1002", testkit.Mod{PackageID: "tester.beta", Name: "Beta"})
	e.MustRun("scan")
	if h := api.Home(); !h.CheckLastRun || !h.LastRunPending {
		t.Fatalf("after a Mod change: want the first read to leave the log pending, got %+v", h)
	}
	if !api.ReadNewLog() {
		t.Fatal("did not refresh the stale report after the page read")
	}
	if h := api.Home(); h.LastRun == "" || h.LastRunPending {
		t.Fatalf("after refreshing the report: last run %q, pending %v; want the log read again", h.LastRun, h.LastRunPending)
	}
}

// When Player.log is gone and only Player-prev.log is left, Can it run reads the previous
// file (06 §4) and Launch names that run too: it showed no last game line at all (VM round
// 2A 2026-11-23, an own-saves Game Config whose Player.log was deleted).
func TestHomeReadsThePreviousLogWhenTheCurrentIsGone(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.alpha", Name: "Alpha Props", Files: map[string]string{"Defs/Test.xml": "<Defs><ThingDef><defName>AlphaDef</defName></ThingDef></Defs>"}})
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.alpha")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.App.LazyLog, e.App.LogCacheOnly = true, true
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	e.RecordLaunch(e.Now().Add(-time.Minute))
	cur := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(cur, "RimWorld 1.6.4633 rev1\n\n"+
		strings.Repeat("Could not load Texture2D at 'Things/Alpha' for def 'AlphaDef'\n\n", 3))
	if !api.ReadNewLog() {
		t.Fatal("did not read the new log")
	}
	if h := api.Home(); h.LastRun == "" {
		t.Fatal("no last run after reading the log")
	}
	if err := os.Rename(cur, filepath.Join(e.UserData, "Player-prev.log")); err != nil {
		t.Fatal(err)
	}
	if h := api.Home(); !h.CheckLastRun {
		t.Fatalf("the previous log should be queued for background analysis: %+v", h)
	}
	if !api.ReadNewLog() {
		t.Fatal("did not analyse the previous log")
	}
	if h := api.Home(); h.LastRun == "" && h.MissingLog == "" {
		t.Fatalf("Launch says nothing of the last run with Player-prev.log left: %+v", h)
	}
}

// A 1.6 Game Config on the shared default folder whose oversized Player.log is deleted from
// the size badge: Player-prev.log is its last run, never the stale 1.7 Logs/Player.log a
// RimWorld 1.7 run left in the same folder without a RimArcho launch record. Launch named
// nothing (an empty placeholder), and Game log and `archo log` read the 1.7 file (release
// 1.1.4 VM round, bug 2).
func TestDeletedPlayerLogFallsBackToOwnVersion(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.alpha", Name: "Alpha Props", Files: map[string]string{"Defs/Test.xml": "<Defs><ThingDef><defName>AlphaDef</defName></ThingDef></Defs>"}})
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.alpha")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.App.LazyLog, e.App.LogCacheOnly = true, true
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	// a 1.7 run written outside RimArcho, days ago
	stale := filepath.Join(e.UserData, "Logs", "Player.log")
	e.WriteFile(stale, "=== Waylith log ===\n2026-11-21 09:00:00.000  INFO [Verse] RimWorld 1.7.5014 rev1\n"+
		"2026-11-21 09:00:01.000 ERROR [Verse] Exception ticking WaylithOnly: System.NullReferenceException: 1.7-only\n")
	old := e.Now().Add(-96 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	e.RecordLaunch(e.Now().Add(-time.Hour))
	prev := filepath.Join(e.UserData, "Player-prev.log")
	e.WriteFile(prev, "RimWorld 1.6.4633 rev1\n\n"+
		strings.Repeat("Could not load Texture2D at 'Things/Alpha' for def 'AlphaDef'\n\n", 3))
	// the run before the last launch (the game moved it aside when it started)
	if err := os.Chtimes(prev, e.Now().Add(-2*time.Hour), e.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	cur := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(cur, "RimWorld 1.6.4633 rev1\n\nOversized run\n\n")
	if err := os.Truncate(cur, app.LogSizeLimit+1); err != nil {
		t.Fatal(err)
	}
	p, err := api.PreviewLogCleanup(cur, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := api.ApplyLogCleanup(cur, p.Token, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cur); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Player.log not deleted: %v", err)
	}

	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if c, pr := e.App.LogPaths(st); c == stale || pr == stale {
		t.Fatalf("the 1.7 log counts as this 1.6 Game Config's: cur %q prev %q", c, pr)
	}
	e.App.LazyLog = false
	if got := e.App.ProblemLog(st); got != prev {
		t.Fatalf("problem log %q, want Player-prev.log", got)
	}
	e.App.LazyLog = true
	// as home.js does: a pending last run asks ReadNewLog, then Launch draws again
	if h := api.Home(); h.CheckLastRun {
		api.ReadNewLog()
	}
	if h := api.Home(); h.LastRunPending {
		t.Fatalf("Launch keeps reading the log: %+v", h)
	} else if h.LastRun == "" && h.MissingLog == "" {
		t.Fatalf("Launch says nothing of the last run: %+v", h)
	} else if strings.Contains(h.LastRun, "WaylithOnly") || strings.Contains(h.MissingLog, "Logs") {
		t.Fatalf("Launch reads the 1.7 log: %+v", h)
	}
	page, err := view.BuildLogPage(e.App, st, "", nil)
	if err != nil || page.Path != prev {
		t.Fatalf("Game log opens %q (%v), want Player-prev.log", page.Path, err)
	}
	out := e.MustRun("log")
	if strings.Contains(out, "WaylithOnly") || strings.Contains(out, "1.7") {
		t.Fatalf("archo log reads the 1.7 log:\n%s", out)
	}
	if out := e.MustRun("status"); strings.Contains(out, "stale log") || strings.Contains(out, "1.7") {
		t.Fatalf("archo status names the 1.7 log:\n%s", out)
	}

	// only another game version's log left (a 1.5 run before the game was updated): Launch
	// says the log was not found instead of an empty line
	e.WriteFile(prev, "RimWorld 1.5.4104 rev1\n\nOld run\n\n")
	e.App.DropLogCaches()
	api.ReadNewLog()
	if h := api.Home(); h.LastRun != "" || !strings.Contains(h.MissingLog, "not found") {
		t.Fatalf("Launch with only a 1.5 log: last run %q, missing %q", h.LastRun, h.MissingLog)
	}
}
