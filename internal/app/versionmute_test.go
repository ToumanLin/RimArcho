package app_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func versionMuteEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.5000 rev1")
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.a", Name: "Alpha", Supported: []string{"1.6"}})
	e.AddWorkshop("1002", testkit.Mod{PackageID: "tester.b", Name: "Beta", Supported: []string{"1.5", "1.6"}})
	e.WriteModsConfig("1.7.5000 rev1", "ludeon.rimworld", "tester.a", "tester.b")
	e.AddEnv(false)
	e.MustRun("scan")
	return e
}

func versionProblemCounts(t *testing.T, e *testkit.Env) (active, muted int) {
	t.Helper()
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps.List {
		if p.Code == "version.unsupported" {
			active++
		}
	}
	return active, len(ps.Muted)
}

func TestVersionMuteMigrationAndStorage(t *testing.T) {
	e := versionMuteEnv(t)
	e.DowngradeFacts()
	if _, err := e.App.Store.DB.Exec(`DROP TABLE version_mute; UPDATE meta SET value='22' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.SetMeta("keeper", "preserved"); err != nil {
		t.Fatal(err)
	}
	e.Open()
	if e.App.Store.Meta("schema_version") != fmt.Sprint(store.SchemaVersion) || e.App.Store.Meta("keeper") != "preserved" {
		t.Fatal("migration lost state")
	}
	if err := e.App.Store.SetVersionMutes([]string{" TESTER.A_Steam ", "tester.a"}, "1.7", true); err != nil {
		t.Fatal(err)
	}
	e.Open()
	got, err := e.App.Store.VersionMutes("1.7")
	if err != nil || !reflect.DeepEqual(got, map[string]bool{"tester.a": true}) {
		t.Fatalf("stored: %v %v", got, err)
	}
	if active, muted := versionProblemCounts(t, e); active != 1 || muted != 1 {
		t.Fatalf("counts %d %d", active, muted)
	}
	if err := e.App.Store.SetVersionMutes([]string{"TESTER.A_STEAM"}, "1.7", false); err != nil {
		t.Fatal(err)
	}
	if active, muted := versionProblemCounts(t, e); active != 2 || muted != 0 {
		t.Fatalf("unmute %d %d", active, muted)
	}
}

func TestVersionMuteAcrossPlansGamesUpdatesAndCopies(t *testing.T) {
	e := versionMuteEnv(t)
	e.MustRun("problems", "mute-version", "pkg:tester.a")
	e.MustRun("plan", "new", "other", "--from", "current")
	e.MustRun("plan", "use", "other")
	if active, muted := versionProblemCounts(t, e); active != 1 || muted != 1 {
		t.Fatalf("other plan %d %d", active, muted)
	}
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.a", Name: "Alpha updated", Supported: []string{"1.5", "1.6"}})
	e.MustRun("scan")
	if active, muted := versionProblemCounts(t, e); active != 1 || muted != 1 {
		t.Fatalf("updated %d %d", active, muted)
	}
	game := filepath.Join(e.Root, "OtherGame")
	e.WriteFile(filepath.Join(game, "RimWorldWin64.exe"), "")
	e.WriteFile(filepath.Join(game, "Version.txt"), "1.7.6000 rev2")
	e.CopyTemplate("core", filepath.Join(game, "Data", "Core"))
	e.WriteMod(filepath.Join(game, "Mods", "A"), testkit.Mod{PackageID: "TESTER.A", Name: "Independent copy", Supported: []string{"1.6"}})
	e.MustRun("env", "add", "second", "--game", game, "--user-data", filepath.Join(e.Root, "OtherData"))
	e.MustRun("env", "use", "second")
	e.MustRun("scan")
	e.MustRun("plan", "new", "copy")
	e.MustRun("plan", "use", "copy")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var copyRef string
	for _, m := range st.Ix.Mods {
		if m.Name == "Independent copy" {
			copyRef = st.Ix.Ref(m.ID)
		}
	}
	if copyRef == "" {
		t.Fatal("copy not scanned")
	}
	e.MustRun("enable", copyRef)
	if active, muted := versionProblemCounts(t, e); active != 0 || muted != 1 {
		t.Fatalf("other game and copy %d %d", active, muted)
	}
	e.App.GameVersion = "1.8"
	if active, muted := versionProblemCounts(t, e); active != 1 || muted != 0 {
		t.Fatalf("1.8 %d %d", active, muted)
	}
	e.App.GameVersion = ""
	e.Open()
	if active, muted := versionProblemCounts(t, e); active != 0 || muted != 1 {
		t.Fatalf("reopened %d %d", active, muted)
	}
	e.MustRun("problems", "unmute-version", copyRef)
	if active, muted := versionProblemCounts(t, e); active != 1 || muted != 0 {
		t.Fatalf("restored %d %d", active, muted)
	}
}

func TestVersionMuteCountsAndAtomicValidation(t *testing.T) {
	e := versionMuteEnv(t)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	before, err := e.App.PlanVersionsOf(st, st.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.App.SetVersionMute([]string{"pkg:tester.a", "#not-a-mod"}, "1.7", true); err == nil {
		t.Fatal("accepted missing ref")
	}
	if active, muted := versionProblemCounts(t, e); active != 2 || muted != 0 {
		t.Fatal("partially wrote invalid batch")
	}
	if err := e.App.SetVersionMute([]string{"pkg:tester.a"}, "1.7.1", true); err == nil {
		t.Fatal("accepted full version")
	}
	e.MustRun("problems", "mute-version", "pkg:tester.a", "pkg:tester.b")
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range app.LaunchWarningsOf(ps) {
		if w.Check.Code == "version.unsupported" {
			t.Fatal("launch counts muted warnings")
		}
	}
	pv, err := e.App.PlanVersionsOf(ps.State, ps.State.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Missing["1.7"] != 0 || pv.Muted["1.7"] != 2 || !reflect.DeepEqual(before.Common, pv.Common) || !reflect.DeepEqual(before.Default, pv.Default) {
		t.Fatalf("plan versions: %+v", pv)
	}
	p, err := e.App.BuildPack("muted-pack", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Mismatch) != 0 {
		t.Fatalf("pack warnings: %v", p.Mismatch)
	}
	if got := e.MustRun("plan", "versions"); !strings.Contains(got, "2 version warnings ignored") || strings.Contains(got, "1.7  every mod lists it") {
		t.Fatalf("plan output: %s", got)
	}
}
