package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestSaveDataArg(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, want string }{
		{`"D:\Games\RimWorld\RimWorldWin64.exe"`, ``},
		{`RimWorldWin64.exe -savedatafolder=D:\packs\a\userdata -popupwindow`, `D:\packs\a\userdata`},
		{`RimWorldWin64.exe "-savedatafolder=C:\My Packs\b c\userdata" -popupwindow`, `C:\My Packs\b c\userdata`},
		{`RimWorldWin64.exe -SaveDataFolder="C:\x y\userdata"`, `C:\x y\userdata`},
	} {
		if got := app.SaveDataArg(c.in); got != c.want {
			t.Errorf("SaveDataArg(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A running game is the environment's list or a pack (10 §3): by its -savedatafolder,
// else by the launch records.
func TestWhatRuns(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	a := e.App
	en, err := a.Env()
	if err != nil {
		t.Fatal(err)
	}
	ud := filepath.Join(a.PacksDir(), "Big Pack", "userdata")
	if err := os.MkdirAll(ud, 0o755); err != nil {
		t.Fatal(err)
	}
	now := a.Now()
	if w := a.WhatRuns(en, &app.GameRun{ArgsKnown: true, Started: now}); w.Pack != "" || w.UserData != a.DefaultUserData(en.GameRoot) {
		t.Fatalf("default data folder: %+v", w)
	}
	if w := a.WhatRuns(en, &app.GameRun{ArgsKnown: true, SaveData: ud, Started: now}); w.Pack != "Big Pack" || w.Guess {
		t.Fatalf("pack data folder: %+v", w)
	}
	// A test run's game is not the player's (VM 2026-11-10: status said "running the list").
	tr := &app.RunningGame{Env: en, Run: &app.GameRun{ArgsKnown: true, PID: 7, SaveData: filepath.Join(a.Home, "tests", en.ID, "userdata")}}
	if tr.What = a.WhatRuns(en, tr.Run); !tr.What.Test || tr.What.Pack != "" {
		t.Fatalf("test run data folder: %+v", tr.What)
	}
	if d := tr.Describe(nil); d != "Game Config "+en.Name+" is running a test run (pid 7)" {
		t.Fatal(d)
	}
	// The command line unreadable: the launch records decide.
	if w := a.WhatRuns(en, &app.GameRun{Started: now}); w.Pack != "" {
		t.Fatalf("no launch: %+v", w)
	}
	if err := a.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: en.ID, Started: now.Add(-time.Minute), Method: "pack:Big Pack", Command: "x"}); err != nil {
		t.Fatal(err)
	}
	if w := a.WhatRuns(en, &app.GameRun{Started: now}); w.Pack != "Big Pack" || !w.Guess {
		t.Fatalf("pack launch record: %+v", w)
	}
	if got := a.RunOfLog(en.ID, now); got != "Big Pack" {
		t.Fatalf("RunOfLog = %q", got)
	}
	if err := a.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: en.ID, Started: now.Add(-30 * time.Second), Method: "steam", Command: "x", PlanID: a.Store.CurrentPlanID(en.ID)}); err != nil {
		t.Fatal(err)
	}
	if w := a.WhatRuns(en, &app.GameRun{Started: now}); w.Pack != "" {
		t.Fatalf("list launched last: %+v", w)
	}
}

// The store can represent siblings before their registration workflow is enabled.
func TestRunningSiblingAttribution(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	main, _ := e.App.Env()
	main.UserDataRoot = e.App.DefaultUserData(main.GameRoot)
	if err := e.App.Store.UpdateEnv(main); err != nil {
		t.Fatal(err)
	}
	sibling := *main
	sibling.ID, sibling.Name, sibling.UserDataRoot = e.App.NewID(), "sibling", filepath.Join(e.Root, "sibling-data")
	if err := e.App.Store.InsertEnv(&sibling); err != nil {
		t.Fatal(err)
	}
	e.App.EnvName = sibling.Name
	for _, tc := range []struct {
		root  string
		known bool
		want  string
	}{
		{"", true, main.Name}, {sibling.UserDataRoot, true, sibling.Name},
		{filepath.Join(e.Root, "unknown"), true, ""}, {"", false, ""},
	} {
		e.App.GameProbe = func(string) *app.GameRun {
			return &app.GameRun{PID: 42, ArgsKnown: tc.known, SaveData: tc.root, Started: e.Now()}
		}
		g, err := e.App.RunningGame()
		if err != nil || g == nil {
			t.Fatalf("probe: %+v %v", g, err)
		}
		if tc.want == "" {
			if !g.UnknownEnv || !strings.Contains(g.Describe(nil), "Game Config unknown") {
				t.Fatalf("guessed sibling: %+v", g)
			}
		} else if g.UnknownEnv || g.Env.Name != tc.want {
			t.Fatalf("wanted %s: %+v", tc.want, g)
		}
	}
	r := &store.LaunchRecord{EnvID: main.ID, GameID: main.GameID, EnvName: main.Name, UserDataRoot: main.UserDataRoot, Method: "direct", Started: e.Now().Add(-time.Second)}
	if err := e.App.Store.AddLaunchRecord(r); err != nil {
		t.Fatal(err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42, Started: e.Now()} }
	g, err := e.App.RunningGame()
	if err != nil || g.UnknownEnv || g.Env.ID != main.ID {
		t.Fatalf("launch fallback: %+v %v", g, err)
	}
	main.UserDataRoot = filepath.Join(e.Root, "changed-data")
	if err := e.App.Store.UpdateEnv(main); err != nil {
		t.Fatal(err)
	}
	ls, err := e.App.Store.Launches(main.ID, 1)
	if err != nil || len(ls) != 1 || ls[0].UserDataRoot == main.UserDataRoot || ls[0].GameID != main.GameID || ls[0].EnvName != "main" {
		t.Fatalf("launch snapshot: %+v %v", ls, err)
	}
}
