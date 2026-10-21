package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
)

// Launching writes the list (08 §2): no "unsaved changes" block, a version with source
// launch (none when nothing changed since the newest version), the launch recorded.
func TestLaunchWritesTheList(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", refOf(t, e, "B", ""))
	before := e.ReadModsConfig()

	out := e.MustRun("launch", "--dry-run")
	for _, want := range []string{"nothing written", "path: ", "entries: 4", "changes: 2 since v1", "history: a new version (source launch)", "command: ", "RimWorldWin64.exe"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry run lacks %q:\n%s", want, out)
		}
	}
	if e.ReadModsConfig() != before || len(e.Launched) != 0 {
		t.Fatal("the dry run wrote or launched")
	}

	out = e.MustRun("launch")
	if got := e.ReadModsConfig(); !strings.Contains(got, "<li>tester.b</li>") || strings.Contains(got, "gone.mod") {
		t.Fatalf("ModsConfig.xml not written:\n%s", got)
	}
	if len(e.Launched) != 1 || !strings.Contains(out, "history: v2 (launch)") {
		t.Fatalf("launch: %v\n%s", e.Launched, out)
	}
	vs, _ := e.App.Store.Versions(mustPlan(t, e))
	if len(vs) != 2 || vs[0].Source != "launch" {
		t.Fatalf("versions: %d, newest %s", len(vs), vs[0].Source)
	}
	// Our own write is not an external change.
	scanNow(t, e)
	if vs, _ := e.App.Store.Versions(mustPlan(t, e)); len(vs) != 2 {
		t.Fatalf("rescan after launch made %d versions", len(vs))
	}
	st, _ := e.App.Loadout()
	if len(st.Changes()) != 0 || st.WS.OriginVersion != 2 {
		t.Fatalf("workspace after launch: %d changes, origin v%d", len(st.Changes()), st.WS.OriginVersion)
	}

	// Again, nothing changed: no new version, a second launch record.
	e.Advance(time.Hour)
	out = e.MustRun("launch")
	if !strings.Contains(out, "same as v2, no new version") {
		t.Fatal(out)
	}
	if vs, _ := e.App.Store.Versions(mustPlan(t, e)); len(vs) != 2 {
		t.Fatalf("an unchanged launch recorded a version (%d)", len(vs))
	}
	var n int
	e.App.Store.DB.QueryRow(`SELECT count(*) FROM launch`).Scan(&n)
	if n != 2 || len(e.Launched) != 2 {
		t.Fatalf("launch records %d, launched %d", n, len(e.Launched))
	}
}

// A launch that cannot start (no executable) writes nothing.
func TestLaunchRefusesBeforeWriting(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("enable", refOf(t, e, "B", ""))
	before := e.ReadModsConfig()
	os.Remove(filepath.Join(e.Game, "RimWorldWin64.exe"))
	if out, code := e.Run("launch"); code == 0 || !strings.Contains(out, "no game executable") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if e.ReadModsConfig() != before {
		t.Fatal("a refused launch wrote ModsConfig.xml")
	}
}

// One game at a time (08 §1, §4): a game running in any registered environment blocks
// launch and pack launch, and the refusal names the environment and what it plays. Saving
// is still allowed and says the game reads it on its next start.
func TestOneGameAtATime(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	other := filepath.Join(e.Root, "Other", "RimWorld")
	e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
	e.WriteFile(filepath.Join(other, "Version.txt"), "1.6.4633 rev1261\n")
	e.MustRun("env", "add", "second", "--game", other)
	e.MustRun("env", "use", "main")
	second, err := e.App.Store.EnvByName("second")
	if err != nil || second == nil {
		t.Fatal(err)
	}

	// The fake process table: one game, running from the second environment's folder.
	running := map[string]*app.GameRun{}
	e.App.GameProbe = func(root string) *app.GameRun { return running[env.PathKey(root)] }
	started := e.Now().Add(-5 * time.Minute)
	running[env.PathKey(other)] = &app.GameRun{PID: 4242, Exe: filepath.Join(other, "RimWorldWin64.exe"), Started: started}

	if r, _ := e.App.GameRunning(); r != nil {
		t.Fatal("GameRunning is the current environment's game only")
	}
	g, err := e.App.RunningGame()
	if err != nil || g == nil || g.Env.Name != "second" || g.Run.PID != 4242 {
		t.Fatalf("RunningGame = %+v, %v", g, err)
	}
	before := e.ReadModsConfig()
	for _, args := range [][]string{{"launch"}, {"launch", "--dry-run"}, {"pack", "launch", "any"}} {
		out, code := e.Run(args...)
		if code == 0 || !strings.Contains(out, "a game is already running") || !strings.Contains(out, "environment second is running its list (pid 4242") {
			t.Fatalf("%v: exit %d\n%s", args, code, out)
		}
	}
	if _, err := e.App.Launch(false); !errors.Is(err, app.ErrLaunchBlocked) {
		t.Fatalf("Launch error %v does not wrap ErrLaunchBlocked", err)
	}
	if e.ReadModsConfig() != before || len(e.Launched) != 0 {
		t.Fatal("a refused launch wrote or launched")
	}

	// The command line unreadable: the launch records say it is a pack.
	if err := e.App.Store.AddLaunch(second.ID, started.Add(-time.Minute), "pack:Raid Night", "x"); err != nil {
		t.Fatal(err)
	}
	if out, _ := e.Run("launch"); !strings.Contains(out, "environment second is running pack Raid Night (from the launch records)") {
		t.Fatal(out)
	}

	// Saving is allowed; the second environment's game does not read this file.
	e.MustRun("enable", refOf(t, e, "B", ""))
	if out := e.MustRun("save"); strings.Contains(out, "next start") {
		t.Fatalf("a game of another data folder was reported:\n%s", out)
	}
	// This environment's game runs: saving still writes, and says when the game reads it.
	delete(running, env.PathKey(other))
	running[env.PathKey(e.Game)] = &app.GameRun{PID: 7, Exe: filepath.Join(e.Game, "RimWorldWin64.exe"), Started: started, ArgsKnown: true}
	e.MustRun("disable", refOf(t, e, "B", ""))
	out := e.MustRun("save")
	if !strings.Contains(out, "it reads the new list on its next start") || strings.Contains(e.ReadModsConfig(), "tester.b") {
		t.Fatalf("save while running:\n%s", out)
	}
	if out, code := e.Run("launch"); code == 0 || !strings.Contains(out, "environment main is running its list (pid 7") {
		t.Fatalf("exit %d\n%s", code, out)
	}

	// Nothing running: the launch goes through.
	delete(running, env.PathKey(e.Game))
	e.MustRun("launch")
	if len(e.Launched) != 1 {
		t.Fatalf("launched %v", e.Launched)
	}
}
