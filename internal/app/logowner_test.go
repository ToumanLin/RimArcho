package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func addSharedLogGame(t *testing.T, e *testkit.Env, version string) string {
	t.Helper()
	root := filepath.Join(e.Root, "other-game")
	e.WriteFile(filepath.Join(root, "RimWorldWin64.exe"), "")
	e.WriteFile(filepath.Join(root, "Version.txt"), version+"\n")
	e.MustRun("env", "add", "other", "--game", root, "--user-data", e.UserData)
	e.MustRun("env", "use", "main")
	return root
}

func writeLogAt(t *testing.T, e *testkit.Env, path, text string, at time.Time) {
	t.Helper()
	e.WriteFile(path, text)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// No foreign log may enter even a stale group, hit a cached report, mark itself seen,
// prompt a new log, or start the other version's Def/Patch analysis (03 §6.4).
func TestForeignLogExcludedBeforeAnalysis(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	text := e.HarmonyLogFixture()
	cur := filepath.Join(e.UserData, "Player.log")
	e.MustRun("log") // a report already exists at the shared path
	addSharedLogGame(t, e, "1.7.4979 rev109")
	e.MustRun("analyze")
	other, _ := e.App.Store.EnvByName("other")
	if err := e.App.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: other.ID, Started: e.Now().Add(-5 * time.Minute), Method: "direct"}); err != nil {
		t.Fatal(err)
	}
	foreign := "=== Waylith log ===\n2026-10-28 12:00:00.000  INFO [Verse] RimWorld 1.7.4979 rev109\n" +
		"2026-10-28 12:00:01.000 ERROR [Verse] Exception ticking ForeignPawn: System.NullReferenceException: foreign-only\n"
	writeLogAt(t, e, cur, foreign, e.Now())
	writeLogAt(t, e, filepath.Join(e.UserData, "Logs", "Player.log"), foreign, e.Now())
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	// A selection saved before the file was overwritten must not bypass the filter.
	if err := e.App.Store.SetMeta("log.selected:"+st.Ix.Env.ID, cur); err != nil {
		t.Fatal(err)
	}
	runs := 0
	e.App.AnalyzeRunner = func(string, func(string)) error { runs++; return fmt.Errorf("unexpected analysis") }
	seen := e.App.Store.Meta("log.seen:" + st.Ix.Env.ID)
	if err := e.App.SelectLog(st, cur); err == nil || e.App.Store.Meta("log.seen:"+st.Ix.Env.ID) != seen {
		t.Fatal("foreign log was selected or marked seen")
	}
	if _, ok := e.App.NewLog(st); ok {
		t.Fatal("foreign log prompted as new")
	}
	if _, err := e.App.CachedLog(cur); err == nil {
		t.Fatal("foreign log cache was used")
	}
	if err := e.App.EnsureLogFileAnalysis(st, cur, nil); err == nil {
		t.Fatal("foreign analysis allowed")
	}
	for _, flags := range [][]string{nil, {"--file", cur}, {"--game-version", "1.7"}, {"--file", cur, "--game-version", "1.7"}, {"--file", cur, "--select"}, {"--follow", "--once"}, {"--follow", "--once", "--file", cur}} {
		if out, code := e.Run(append([]string{"log"}, flags...)...); code == 0 {
			t.Fatalf("foreign log accepted %v: %s", flags, out)
		}
	}
	for _, lazy := range []bool{false, true} {
		e.App.LazyLog = lazy
		if p := e.App.ProblemLog(st); p != "" {
			t.Fatalf("foreign problem log: %s", p)
		}
		ps, err := e.App.Problems(false, nil)
		if err != nil || len(ps.Logs) != 0 || logProblems(ps.All()) != 0 {
			t.Fatalf("foreign problems: %+v, %v", ps, err)
		}
		n := view.BuildNeeds(e.App, ps.State, ps)
		if n.LastRun != "" {
			t.Fatalf("foreign last-run errors: %s", n.LastRun)
		}
	}
	e.App.LazyLog = false
	for _, argv := range [][]string{{"problems", "--all"}, {"status"}} {
		if out := e.MustRun(argv...); strings.Contains(out, "ForeignPawn") || strings.Contains(out, "stale log") {
			t.Fatal(out)
		}
	}
	p, err := view.BuildLogPage(e.App, st, cur, nil)
	if err != nil || len(p.Files) != 0 || len(p.Rows) != 0 || p.Path != "" || runs != 0 {
		t.Fatalf("foreign log page %+v, %v, %d analyses", p, err, runs)
	}
	var count int
	if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM analysis WHERE environment_id=? AND game_mm='1.7'`, st.Ix.Env.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("foreign analysis slots: %d, %v", count, err)
	}

	// The latest file is foreign, but the preceding run is ours; find it even if it
	// exists only in the other layout's directory.
	prev := filepath.Join(e.UserData, "Logs", "Player-prev.log")
	writeLogAt(t, e, prev, text, e.Now().Add(-20*time.Minute))
	if got := e.App.ProblemLog(st); got != prev {
		t.Fatalf("own previous log not chosen: %q", got)
	}
	e.App.LazyLog = true
	if got := e.App.ProblemLog(st); got != prev {
		t.Fatalf("sidebar did not fall back to own previous log: %q", got)
	}
	e.App.LazyLog = false
	if out := e.MustRun("log"); !strings.Contains(out, "Player-prev.log") || strings.Contains(out, "foreign-only") {
		t.Fatal(out)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil || len(ps.Logs) != 1 || ps.Logs[0].Path != prev || logProblems(ps.List) != 4 {
		t.Fatalf("own previous run %+v, %v", ps, err)
	}
	p, err = view.BuildLogPage(e.App, st, "", nil)
	if err != nil || p.Path != prev || len(p.Files) != 1 || len(p.Rows) == 0 || runs != 0 {
		t.Fatalf("own previous page %+v, %v, %d analyses", p, err, runs)
	}
}

func TestLogOwnerEvidence(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	text := e.HarmonyLogFixture()
	other := addSharedLogGame(t, e, "1.7.4979 rev109")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(e.UserData, "Player.log")
	mono := func(root string) string {
		return "Mono path[0] = '" + filepath.ToSlash(filepath.Join(root, "RimWorldWin64_Data", "Managed")) + "'\n"
	}
	// A concrete startup path beats the most recent (but no longer relevant) launch.
	writeLogAt(t, e, p, mono(other)+text, e.Now())
	if err := e.App.CheckLogOwner(st, p); err == nil || !strings.Contains(err.Error(), "other") {
		t.Fatalf("foreign game path ignored: %v", err)
	}
	e.App.Store.DB.Exec(`DELETE FROM launch`)
	// A mismatched version alone is not evidence of another game: upgrades are allowed,
	// even when another registered environment currently uses that version.
	writeLogAt(t, e, p, strings.ReplaceAll(text, "1.6.4633 rev1261", "1.7.4979 rev109"), e.Now())
	if err := e.App.CheckLogOwner(st, p); err != nil {
		t.Fatalf("unknown owner mistaken for foreign: %v", err)
	}
	calls := 0
	e.App.AnalyzeRunner = func(string, func(string)) error { calls++; return fmt.Errorf("unexpected analysis") }
	if err := e.App.EnsureLogFileAnalysis(st, p, nil); err != nil || calls != 0 {
		t.Fatalf("unknown owner triggered version analysis: %d, %v", calls, err)
	}
	r := e.App.LogRunOf(st, p)
	if !r.Unknown || !r.Stale() {
		t.Fatalf("unknown log must remain uncounted: %+v", r)
	}
	second, _ := e.App.Store.EnvByName("other")
	if err := e.App.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: second.ID, Started: e.Now().Add(-time.Minute), Method: "direct"}); err != nil {
		t.Fatal(err)
	}
	writeLogAt(t, e, p, mono(e.Game)+text, e.Now())
	if err := e.App.CheckLogOwner(st, p); err != nil {
		t.Fatalf("own startup path lost to another launch: %v", err)
	}
	if r := e.App.LogRunOf(st, p); !r.Unknown || r.OtherRun {
		t.Fatalf("contradictory launch assigned a plan: %+v", r)
	}
	// Following a shared path must discard clusters if its owner changes.
	lv := e.App.FollowLog(p, "")
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	writeLogAt(t, e, p, mono(other)+text, e.Now())
	cs, err := lv.Poll(e.App)
	if err != nil || len(cs) != 0 || len(lv.Clusters()) != 0 {
		t.Fatalf("foreign live clusters retained: %d, %v", len(cs), err)
	}
}

func TestOwnLogBeforeGameUpgrade(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	text := e.HarmonyLogFixture()
	p := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(p, "Mono path[0] = '"+filepath.ToSlash(filepath.Join(e.Game, "RimWorldWin64_Data", "Managed"))+"'\n"+text)
	addSharedLogGame(t, e, "1.6.4633 rev1261")
	// Upgrade this game directory to 1.7 while its old 1.6 log remains at the old path.
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.4979 rev109\n")
	e.MustRun("scan")
	runs := []string{}
	inner := e.App.AnalyzeRunner
	e.App.AnalyzeRunner = func(name string, progress func(string)) error {
		runs = append(runs, e.App.GameVersion)
		return inner(name, progress)
	}
	out := e.MustRun("log")
	if !strings.Contains(out, "analysed: as 1.6 (log version); environment is 1.7") || len(runs) != 1 || runs[0] != "1.6" {
		t.Fatalf("old own log did not use 1.6 analysis: runs %v\n%s", runs, out)
	}
}
