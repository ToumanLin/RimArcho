package app_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func TestSaveDataArg(t *testing.T) {
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

// A running game is the environment's list or a pack (10 §8.3): by its -savedatafolder,
// else by the launch records.
func TestWhatRuns(t *testing.T) {
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
	if w := a.WhatRuns(en, &app.GameRun{ArgsKnown: true, Started: now}); w.Pack != "" || w.UserData != en.UserDataRoot {
		t.Fatalf("default data folder: %+v", w)
	}
	if w := a.WhatRuns(en, &app.GameRun{ArgsKnown: true, SaveData: ud, Started: now}); w.Pack != "Big Pack" || w.Guess {
		t.Fatalf("pack data folder: %+v", w)
	}
	// The command line unreadable: the launch records decide.
	if w := a.WhatRuns(en, &app.GameRun{Started: now}); w.Pack != "" {
		t.Fatalf("no launch: %+v", w)
	}
	if err := a.Store.AddLaunch(en.ID, now.Add(-time.Minute), "pack:Big Pack", "x"); err != nil {
		t.Fatal(err)
	}
	if w := a.WhatRuns(en, &app.GameRun{Started: now}); w.Pack != "Big Pack" || !w.Guess {
		t.Fatalf("pack launch record: %+v", w)
	}
	if got := a.RunOfLog(en.ID, now); got != "Big Pack" {
		t.Fatalf("RunOfLog = %q", got)
	}
	if err := a.Store.AddLaunch(en.ID, now.Add(-30*time.Second), "steam", "x"); err != nil {
		t.Fatal(err)
	}
	if w := a.WhatRuns(en, &app.GameRun{Started: now}); w.Pack != "" {
		t.Fatalf("list launched last: %+v", w)
	}
}
