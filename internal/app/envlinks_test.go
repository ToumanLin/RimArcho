package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func envLinksFixture(t *testing.T) (*testkit.Env, string) {
	t.Helper()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.b")
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	e.MustRun("monitor", "install")
	e.MustRun("save")
	g2 := secondGame(e, "OtherGame")
	e.MustRun("env", "add", "other", "--game", g2)
	return e, g2
}

func TestEnvUseLinksLocalAndMonitorBeforeImport(t *testing.T) {
	e, g2 := envLinksFixture(t)
	before := e.ReadModsConfig()
	main := active(t, e)
	e.MustRun("env", "use", "other")
	if got := active(t, e); !slices.Equal(got, main) {
		t.Fatalf("switch lost installed mods: %v; want %v", got, main)
	}
	if got := e.ReadModsConfig(); got != before {
		t.Fatal("switch rewrote ModsConfig.xml")
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"B", monName} {
		var found bool
		for _, c := range e.App.ModsCopies(st.Ix) {
			if c.Mod != nil && c.Mod.Name == name {
				found = c.Link && source.IsLink(filepath.Join(g2, "Mods", c.Name))
			}
		}
		if !found {
			t.Fatalf("%s is not linked into the destination", name)
		}
	}
	// Saving and switching again retain the local link and do not duplicate packages.
	e.MustRun("save")
	scanNow(t, e)
	if got := active(t, e); !slices.Equal(got, main) {
		t.Fatalf("save removed an inherited local mod: %v", got)
	}
	e.MustRun("env", "use", "main")
	e.MustRun("env", "use", "other")
	st, _ = e.App.Loadout()
	if dups := app.ModsDuplicates(e.App.ModsCopies(st.Ix)); len(dups) != 0 {
		t.Fatalf("repeated switch duplicated mods: %+v", dups)
	}
}

func TestEnvUseKeepsDestinationLocalCopy(t *testing.T) {
	e, g2 := envLinksFixture(t)
	e.WriteMod(filepath.Join(g2, "Mods", "MyB"), testkit.Sample("tester.b", "Destination B"))
	e.MustRun("env", "use", "other")
	got := active(t, e)
	if !slices.Contains(got, "Destination B") || slices.Contains(got, "B") {
		t.Fatalf("did not retain destination's local copy: %v", got)
	}
	st, _ := e.App.Loadout()
	if dups := app.ModsDuplicates(e.App.ModsCopies(st.Ix)); len(dups) != 0 {
		t.Fatalf("destination has duplicates: %+v", dups)
	}
}

func TestEnvUseDoesNotInheritIntoEmptyOwnSaves(t *testing.T) {
	e, g2 := envLinksFixture(t)
	e.MustRun("env", "set", "other", "--saves", "own")
	e.MustRun("env", "use", "other")
	entries, err := os.ReadDir(filepath.Join(g2, "Mods"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("empty separate list inherited links: %v, %v", entries, err)
	}
}

func TestEnvUseRollsBackFailedLinks(t *testing.T) {
	e, g2 := envLinksFixture(t)
	create := app.CreateLink
	t.Cleanup(func() { app.CreateLink = create })
	var calls int
	app.CreateLink = func(link, target string) error {
		calls++
		if calls == 2 {
			return errors.New("test link failure")
		}
		return create(link, target)
	}
	if _, err := e.App.EnvUse("other"); err == nil || !strings.Contains(err.Error(), "test link failure") {
		t.Fatalf("switch error: %v", err)
	}
	if cur, _ := e.App.Store.CurrentEnv(); cur == nil || cur.Name != "main" {
		t.Fatalf("failed switch changed current environment: %+v", cur)
	}
	entries, err := os.ReadDir(filepath.Join(g2, "Mods"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed switch left links: %v, %v", entries, err)
	}
	if !source.HasAbout(filepath.Join(e.Game, "Mods", "B")) {
		t.Fatal("rollback touched source files")
	}
}

func TestEnvUseDoesNotLinkIntoRunningGame(t *testing.T) {
	e, g2 := envLinksFixture(t)
	e.App.GameProbe = func(root string) *app.GameRun {
		if root == g2 {
			return &app.GameRun{PID: 42}
		}
		return nil
	}
	if _, err := e.App.EnvUse("other"); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("switch to running game: %v", err)
	}
}

func TestEnvUseKeepsSelectedMonitorVersion(t *testing.T) {
	e, g2 := envLinksFixture(t)
	e.App.MonitorBundle = monitorBundle(t, e, "0.10.0")
	e.MustRun("monitor", "install")
	e.MustRun("undo") // deliberately keep 0.9.0 in the source plan
	e.MustRun("save")
	e.MustRun("env", "use", "other")
	if !source.IsLink(filepath.Join(g2, "Mods", "_Archo_monitor_ArchoMonitor-0.9.0")) {
		t.Fatal("switch did not preserve the selected Monitor version")
	}
	if _, err := os.Lstat(filepath.Join(g2, "Mods", "_Archo_monitor_ArchoMonitor-0.10.0")); !os.IsNotExist(err) {
		t.Fatalf("switch linked a different Monitor version: %v", err)
	}
}

func TestEnvUsePreservesLocalWorkshopTokens(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("5100", testkit.Sample("tester.same", "Workshop Same"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Same"), testkit.Sample("tester.same", "Local Same"))
	e.AddEnv(false)
	e.WriteModsConfig("1.6", "ludeon.rimworld", "tester.same", "tester.same_steam")
	scanNow(t, e)
	g2 := secondGame(e, "OtherGame")
	e.MustRun("env", "add", "other", "--game", g2)
	e.MustRun("env", "use", "other")
	if got := active(t, e); !slices.Equal(got, []string{"Core", "Local Same", "Workshop Same"}) {
		t.Fatalf("local and Workshop tokens changed their meaning: %v", got)
	}
}

func TestEnvUseScansExistingDestinationWithoutNewLinks(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	scanNow(t, e)
	g2 := secondGame(e, "OtherGame")
	e.WriteMod(filepath.Join(g2, "Mods", "OnlyHere"), testkit.Sample("tester.here", "Only Here"))
	e.MustRun("env", "add", "other", "--game", g2)
	e.WriteModsConfig("1.6", "ludeon.rimworld", "tester.here")
	e.MustRun("env", "use", "other")
	if got := active(t, e); !slices.Equal(got, []string{"Core", "Only Here"}) {
		t.Fatalf("existing destination mod is missing before its first scan: %v", got)
	}
}
