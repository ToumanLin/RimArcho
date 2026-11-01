package gui_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/surface/gui"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// After a game, the window reads the new Player.log by itself (06 §2.1): Can it run's badge
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
