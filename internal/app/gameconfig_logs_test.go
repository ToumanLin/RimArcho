package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestConfigLogLaunchAndRotation(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.WriteFile(filepath.Join(e.Steam, "steam.exe"), "")
	for _, method := range []string{"direct", "steam"} {
		e.MustRun("env", "set", "main", "--launch", method)
		r, err := e.App.Launch(true)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(r.Launch.Command(), "-logfile") {
			t.Fatal(r.Launch.Command())
		}
	}
	e.MustRun("env", "add", "own", "--like", "main", "--saves", "own")
	e.MustRun("env", "use", "own")
	en, _ := e.App.Env()
	path := filepath.Join(en.UserDataRoot, "Player.log")
	e.WriteFile(path, "previous run")
	e.WriteFile(filepath.Join(en.UserDataRoot, "Player-prev.log"), "older run")
	for _, method := range []string{"direct", "steam"} {
		e.MustRun("env", "set", "own", "--launch", method)
		r, err := e.App.Launch(true)
		if err != nil {
			t.Fatal(err)
		}
		argv := r.Launch.Args
		found := false
		for i, arg := range argv {
			if arg == "-logfile" && i+1 < len(argv) && argv[i+1] == path {
				found = true
			}
		}
		if !found {
			t.Fatalf("own log missing: %v", argv)
		}
		b, _ := os.ReadFile(path)
		if string(b) != "previous run" {
			t.Fatal("dry run rotated log")
		}
	}
	e.MustRun("launch")
	b, err := os.ReadFile(filepath.Join(en.UserDataRoot, "Player-prev.log"))
	if err != nil || string(b) != "previous run" {
		t.Fatalf("rotation: %s %v", b, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("current retained: %v", err)
	}
}

func TestConfigOwnLogResolution(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "add", "own", "--like", "main", "--saves", "own")
	e.MustRun("env", "use", "own")
	st, _ := e.App.Loadout()
	root := st.Ix.Env.UserDataRoot
	current, prev := filepath.Join(root, "Player.log"), filepath.Join(root, "Player-prev.log")
	e.WriteFile(current, "RimWorld 1.6.4633 rev1261\n")
	e.WriteFile(prev, "previous")
	// A later sibling launch cannot take this folder's files away.
	main, _ := e.App.Store.EnvByName("main")
	if err := e.App.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: main.ID, GameID: main.GameID, UserDataRoot: main.UserDataRoot, Method: "direct", Started: e.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(current, e.Now(), e.Now()); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1.6", "1.7"} {
		st.Ix.Env.GameVersion = version
		e.WriteFile(filepath.Join(root, "Logs", "Player.log"), "fallback")
		cur, old := e.App.LogPaths(st)
		if cur != current || old != prev {
			t.Fatalf("%s: %s %s", version, cur, old)
		}
		if err := e.App.CheckLogOwner(st, current); err != nil {
			t.Fatal(err)
		}
	}
	// A pack uses its own explicit file; absence never falls back to the config's log.
	packData := filepath.Join(e.App.PacksDir(), "Synthetic", "userdata")
	w := app.RunWhat{Pack: "Synthetic", UserData: packData}
	if p := e.App.RunLog(st, w); p != "" {
		t.Fatalf("pack borrowed %s", p)
	}
	e.WriteFile(filepath.Join(packData, "Player.log"), "pack")
	if p := e.App.RunLog(st, w); p != filepath.Join(packData, "Player.log") {
		t.Fatal(p)
	}
}

func TestConfigMissingLogIsNotClean(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"1.6", "1.7"} {
		t.Run(version, func(t *testing.T) {
			e := loadoutEnv(t)
			scanNow(t, e)
			e.MustRun("env", "add", "own", "--like", "main", "--saves", "own")
			e.MustRun("env", "use", "own")
			st, _ := e.App.Loadout()
			st.Ix.Env.GameVersion = version
			// Even an error-free default log cannot stand in for the own-folder log.
			e.WriteFile(filepath.Join(e.UserData, "Player.log"), "no errors")
			n := view.BuildNeeds(e.App, st, &app.ProblemSet{State: st})
			want := filepath.Join(st.Ix.Env.UserDataRoot, "Player.log")
			if !strings.Contains(n.MissingLog, "the last run's log was not found") || !strings.Contains(n.MissingLog, want) || n.LastRun != "" {
				t.Fatalf("unsubstantiated clean verdict: %+v", n)
			}
			h := view.BuildHomeFromState(e.App, st, n, nil, nil)
			if h.MissingLog != n.MissingLog {
				t.Fatalf("home lost diagnostic: %+v", h)
			}
		})
	}
}

func TestLogfileOverrideRefused(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	en, _ := e.App.Env()
	for _, args := range []string{"-logfile elsewhere", "-LogFile=elsewhere", `"-logfile=folder with spaces"`, `["-LOGFILE", "elsewhere"]`} {
		out, code := e.Run("env", "set", "main", "--launch-args", args)
		if code == 0 || !strings.Contains(out, "remove -logfile") {
			t.Fatalf("typed: %d %s", code, out)
		}
		if err := e.App.Store.SetMeta("env:"+en.ID+":launch_args", args); err != nil {
			t.Fatal(err)
		}
		before := e.ReadModsConfig()
		for _, launch := range []func() error{
			func() error { _, err := e.App.Launch(false); return err },
			func() error { _, err := e.App.LaunchPack("missing", false); return err },
			func() error { _, err := e.App.TestRun(context.Background(), app.TestRunOptions{}); return err },
		} {
			if err := launch(); err == nil || !strings.Contains(err.Error(), "remove -logfile") {
				t.Fatalf("stored: %v", err)
			}
		}
		if e.ReadModsConfig() != before || len(e.Launched) != 0 {
			t.Fatal("refusal wrote or launched")
		}
	}
}

func TestRedirectedSiblingDoesNotClaimDefaultLog(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("launch")
	main, _ := e.App.Env()
	e.MustRun("env", "add", "own", "--like", "main", "--saves", "own")
	own, _ := e.App.Store.EnvByName("own")
	e.MustRun("env", "use", "own")
	e.MustRun("launch")
	e.MustRun("env", "use", "main")
	st, _ := e.App.Loadout()
	path := filepath.Join(main.UserDataRoot, "Player.log")
	e.WriteFile(path, "RimWorld 1.6.4633 rev1261\n")
	if err := os.Chtimes(path, e.Now(), e.Now()); err != nil {
		t.Fatal(err)
	}
	if err := e.App.CheckLogOwner(st, path); err != nil {
		t.Fatal(err)
	}
	r := logRun(t, e)
	if r.Source.EnvID == own.ID {
		t.Fatalf("redirected sibling stole default log: %+v", r.Source)
	}
}

// Waylith (1.7) ignores -logfile and writes the default folder's Logs/Player.log, so an
// own-folder config reads that file when its own launch is the newest one before it, and
// never when a default-folder sibling ran since (VM round 2026-11-21).
func TestWaylithOwnConfigReadsDefaultLogs(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.WriteFile(filepath.Join(e.Game, "Waylith.dll"), "MZ")
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("env", "add", "own", "--like", "main", "--saves", "own")
	e.MustRun("env", "use", "own")
	e.MustRun("launch")
	st, _ := e.App.Loadout()
	logs := filepath.Join(e.UserData, "Logs")
	cur, prev := filepath.Join(logs, "Player.log"), filepath.Join(logs, "Player-prev.log")
	e.Advance(time.Minute)
	e.WriteFile(cur, "=== Waylith log ===\nVersion:   1.7.5014 rev1195\n")
	e.WriteFile(prev, "older")
	if err := os.Chtimes(cur, e.Now(), e.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(prev, e.Now().Add(-2*time.Hour), e.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, old := e.App.LogPaths(st)
	if got != cur || old != "" {
		t.Fatalf("own run's log not found: %q %q", got, old)
	}
	if err := e.App.CheckLogOwner(st, got); err != nil {
		t.Fatal(err)
	}
	// env list shows it too (VM round 2026-11-22: the line was missing).
	if p := e.App.EnvPlayerLog(st.Ix.Env); p != cur {
		t.Fatalf("env list player-log %q, want %q", p, cur)
	}
	// The default-folder sibling runs next: the file is now its run, not this config's.
	e.Advance(time.Minute)
	e.MustRun("env", "use", "main")
	e.MustRun("launch")
	e.MustRun("env", "use", "own")
	e.Advance(time.Minute)
	if err := os.Chtimes(cur, e.Now(), e.Now()); err != nil {
		t.Fatal(err)
	}
	st, _ = e.App.Loadout()
	if got, old := e.App.LogPaths(st); got != "" || old != "" {
		t.Fatalf("borrowed the sibling's log: %q %q", got, old)
	}
	// A 1.6 own-folder config never looks there.
	e.MustRun("launch")
	e.Advance(time.Minute)
	if err := os.Chtimes(cur, e.Now(), e.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(e.Game, "Waylith.dll")); err != nil {
		t.Fatal(err)
	}
	st, _ = e.App.Loadout()
	st.Ix.Env.GameVersion = "1.6"
	if got, _ := e.App.LogPaths(st); got != "" {
		t.Fatalf("1.6 read Logs/: %q", got)
	}
}

// A 1.7 pack's run also writes the default folder's Logs/Player.log: RunLog follows it
// while the pack's launch is the newest one before it, and never a plan's run.
func TestWaylithPackReadsDefaultLogs(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.WriteFile(filepath.Join(e.Game, "Waylith.dll"), "MZ")
	st, _ := e.App.Loadout()
	main := st.Ix.Env
	packData := filepath.Join(e.App.PacksDir(), "Synthetic", "userdata")
	w := app.RunWhat{Pack: "Synthetic", UserData: packData}
	cur := filepath.Join(e.UserData, "Logs", "Player.log")
	touch := func() {
		e.Advance(time.Minute)
		e.WriteFile(cur, "=== Waylith log ===\nVersion:   1.7.5014 rev1195\n")
		if err := os.Chtimes(cur, e.Now(), e.Now()); err != nil {
			t.Fatal(err)
		}
	}
	launch := func(method, root string) {
		e.Advance(time.Minute)
		if err := e.App.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: main.ID, GameID: main.GameID, UserDataRoot: root, Method: method, Started: e.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	launch("pack:Synthetic", packData)
	touch()
	if p := e.App.RunLog(st, w); p != cur {
		t.Fatalf("pack run's log not found: %q", p)
	}
	// Another pack, then the plan, ran since: neither is this pack's log.
	launch("pack:Other", filepath.Join(e.App.PacksDir(), "Other", "userdata"))
	touch()
	if p := e.App.RunLog(st, w); p != "" {
		t.Fatalf("borrowed another pack's log: %q", p)
	}
	launch("direct", main.UserDataRoot)
	touch()
	if p := e.App.RunLog(st, w); p != "" {
		t.Fatalf("borrowed the plan's log: %q", p)
	}
}

// A 1.6 Game Config in the default folder does not read a 1.7 run's log: a 1.7 install
// shares the folder and always writes Logs/Player.log (VM round 2026-11-22: `archo log
// --env 1.6` read the 1.7 run's log, "analysed: as 1.7 (log version); Game Config is 1.6").
func TestOlderGameSkipsWaylithRunLog(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	g17 := filepath.Join(e.Root, "RimWorld17")
	e.WriteFile(filepath.Join(g17, "RimWorldWin64.exe"), "")
	e.WriteFile(filepath.Join(g17, "Waylith.dll"), "MZ")
	e.WriteFile(filepath.Join(g17, "Version.txt"), "1.7.5014 rev1195")
	e.MustRun("env", "add", "new17", "--game", g17, "--saves", "own")
	e.MustRun("env", "use", "new17")
	e.MustRun("launch")
	e.MustRun("env", "use", "main")
	e.Advance(time.Minute)
	waylith := filepath.Join(e.UserData, "Logs", "Player.log")
	e.WriteFile(waylith, "=== Waylith log ===\nVersion:   1.7.5014.1195\n")
	if err := os.Chtimes(waylith, e.Now(), e.Now()); err != nil {
		t.Fatal(err)
	}
	st, _ := e.App.Loadout()
	if cur, prev := e.App.LogPaths(st); cur != "" || prev != "" {
		t.Fatalf("1.6 read the 1.7 run's log: %q %q", cur, prev)
	}
	if err := e.App.CheckLogOwner(st, waylith); err == nil || !strings.Contains(err.Error(), "new17") {
		t.Fatalf("owner: %v", err)
	}
}
