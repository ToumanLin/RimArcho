package app_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func launches(t *testing.T, e *testkit.Env) []*store.LaunchRecord {
	t.Helper()
	en, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	ls, err := e.App.Store.Launches(en.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return ls
}

// J11: a plan's launch records the plan, the list's fingerprint and the version played; a
// pack's launch has no plan and the pack list's fingerprint (08 §1).
func TestLaunchRecordCarriesPlan(t *testing.T) {
	t.Parallel()
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.b")
	e.MustRun("launch")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	ls := launches(t, e)
	r := ls[0]
	if len(ls) != 1 || r.PlanID != st.Plan.ID || r.Fingerprint != app.ListFingerprint(st.List) || r.VersionN != st.Latest.N || r.VersionN != 2 || r.Pack() != "" {
		t.Fatalf("plan launch: %+v (plan %d, latest v%d)", r, st.Plan.ID, st.Latest.N)
	}
	// Unchanged since: no new version, the record names the version played.
	e.Advance(time.Minute)
	e.MustRun("launch")
	if r := launches(t, e)[0]; r.VersionN != 2 || r.Fingerprint != app.ListFingerprint(st.List) {
		t.Fatalf("unchanged launch: %+v", r)
	}

	e.MustRun("pack", "build", "first")
	e.Advance(time.Minute)
	e.MustRun("pack", "launch", "first")
	r = launches(t, e)[0]
	if r.PlanID != 0 || r.Method != "pack:first" || r.Pack() != "first" || r.Fingerprint == "" || r.Fingerprint == app.ListFingerprint(st.List) {
		t.Fatalf("pack launch: %+v", r)
	}
}

func logRun(t *testing.T, e *testkit.Env) *app.LogRun {
	t.Helper()
	ps := problems(t, e)
	if len(ps.Logs) != 1 {
		t.Fatalf("logs: %+v", ps.Logs)
	}
	return ps.Logs[0]
}

func usePlan(t *testing.T, e *testkit.Env, name string) {
	t.Helper()
	if _, err := e.App.UsePlan(name); err != nil {
		t.Fatal(err)
	}
}

func newPlan(t *testing.T, e *testkit.Env, name string) {
	t.Helper()
	if _, err := e.App.NewPlan(name, "current"); err != nil {
		t.Fatal(err)
	}
}

// 03 §6.4 new rule: a log maps to the newest launch that started no later than it was
// written, and is stale when that launch is another plan's (B9), a pack's or another
// environment's, or when the plan changed since (B10, current again once changed back).
func TestLogBelongsToLaunch(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture() // plan:default launched 30 minutes before the log
	r := logRun(t, e)
	if r.Stale() || r.Source == nil || r.Source.Launch == nil || r.Source.Plan != "default" {
		t.Fatalf("own launch: %+v %+v", r, r.Source)
	}
	if out := e.MustRun("problems"); !strings.Contains(out, "current (from the launch of plan:default at ") {
		t.Fatal(out)
	}

	// B9: the same list in another plan is still another plan's log.
	newPlan(t, e, "other")
	usePlan(t, e, "other")
	r = logRun(t, e)
	if !r.OtherRun || r.Changed || r.Unknown || !r.Stale() || r.Source.Plan != "default" {
		t.Fatalf("another plan: %+v", r)
	}
	if out := e.MustRun("problems"); !strings.Contains(out, "stale log (from another plan: the launch of plan:default at ") {
		t.Fatal(out)
	}
	usePlan(t, e, "default")
	if r := logRun(t, e); r.Stale() {
		t.Fatalf("back on the launched plan: %+v", r)
	}

	// B10: changed since the launch, then changed back.
	e.MustRun("disable", "pkg:tester.manual")
	if r := logRun(t, e); !r.Changed || r.OtherRun || !r.Stale() {
		t.Fatalf("changed since the launch: %+v", r)
	}
	if out := e.MustRun("problems"); !strings.Contains(out, "stale log (the plan changed since the launch of plan:default at ") {
		t.Fatal(out)
	}
	if _, _, err := e.App.UndoRedo(false); err != nil {
		t.Fatal(err)
	}
	if r := logRun(t, e); r.Stale() {
		t.Fatalf("changed back: %+v", r)
	}

	// A pack launched after the plan, before the log was written.
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	pack := &store.LaunchRecord{EnvID: st.Ix.Env.ID, Started: e.Now().Add(-10 * time.Minute).UTC(), Method: "pack:Raid Night", Command: "x"}
	if err := e.App.Store.AddLaunchRecord(pack); err != nil {
		t.Fatal(err)
	}
	if r := logRun(t, e); !r.OtherRun || r.Source.Pack != "Raid Night" || r.OtherEnv != "" {
		t.Fatalf("pack launch: %+v %+v", r, r.Source)
	}
	if out := e.MustRun("problems"); !strings.Contains(out, "stale log (from the launch of pack Raid Night at ") {
		t.Fatal(out)
	}
	e.App.Store.DB.Exec(`DELETE FROM launch WHERE id=?`, pack.ID)

	// Another environment sharing the user data folder launched last.
	other := filepath.Join(e.Root, "Other", "RimWorld")
	e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
	e.WriteFile(filepath.Join(other, "Version.txt"), "1.6.4633 rev1261\n")
	e.MustRun("env", "add", "second", "--game", other, "--user-data", e.UserData)
	e.MustRun("env", "use", "main")
	second, _ := e.App.Store.EnvByName("second")
	if err := e.App.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: second.ID, Started: e.Now().Add(-5 * time.Minute).UTC(), Method: "direct", Command: "x"}); err != nil {
		t.Fatal(err)
	}
	if r := logRun(t, e); !r.OtherRun || r.OtherEnv != "second" {
		t.Fatalf("other environment: %+v", r)
	}
	if out := e.MustRun("problems"); !strings.Contains(out, "in environment second)") {
		t.Fatal(out)
	}
}

// B11 and "来源不明": without a launch record the log belongs to the plan last written to
// the game before it; with neither it is of unknown source, stale, and the header says so.
func TestLogWriteRecordAndUnknown(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.App.Store.DB.Exec(`DELETE FROM launch`)
	r := logRun(t, e)
	if !r.Unknown || !r.Stale() {
		t.Fatalf("no launch, no write: %+v", r)
	}
	out := e.MustRun("problems")
	if !strings.Contains(out, "stale log (source unknown: no launch and no ModsConfig.xml write by RimArcho before it)") {
		t.Fatal(out)
	}
	if strings.Contains(e.MustRun("status"), "current") {
		t.Fatal("status counts an unknown log")
	}

	// Written to the game (archo save) before the session that wrote the log; started from
	// Steam.
	e.MustRun("save")
	r = logRun(t, e)
	if r.Stale() || !r.Source.ByWrite || r.Source.Plan != "default" {
		t.Fatalf("by the write record: %+v %+v", r, r.Source)
	}
	if out := e.MustRun("problems"); !strings.Contains(out, "current (from plan:default as written to the game at ") {
		t.Fatal(out)
	}
	newPlan(t, e, "other")
	usePlan(t, e, "other")
	if r := logRun(t, e); !r.OtherRun || !r.Source.ByWrite {
		t.Fatalf("another plan, by the write record: %+v", r)
	}
	// A write after the log says nothing about it.
	e.Advance(time.Hour)
	e.MustRun("save")
	if r := logRun(t, e); !r.Unknown {
		t.Fatalf("write after the log: %+v", r)
	}
}

// C11: each plan's analysis is kept (03 §3), so switching between two plans with different
// enabled sets and back does not run the analysis again; the per-plan rows read stored
// results only.
func TestAnalysisPerPlan(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	runs := 0
	inner := e.App.AnalyzeRunner
	e.App.AnalyzeRunner = func(env string, p func(string)) error { runs++; return inner(env, p) }
	problems(t, e)
	newPlan(t, e, "other")
	usePlan(t, e, "other")
	e.MustRun("disable", "pkg:tester.manual")
	if ps := problems(t, e); runs != 2 || !ps.Analysis.Fresh {
		t.Fatalf("second plan: %d runs", runs)
	}
	for _, name := range []string{"default", "other", "default"} {
		usePlan(t, e, name)
		if ps := problems(t, e); runs != 2 || !ps.Analysis.Fresh {
			t.Fatalf("back on %s: %d runs, fresh %v", name, runs, ps.Analysis.Fresh)
		}
	}

	// A third enabled set, never analysed: its row is pending; the others are counted
	// from their stored runs. Nothing runs.
	newPlan(t, e, "third")
	usePlan(t, e, "third")
	e.MustRun("disable", "pkg:tester.compat")
	usePlan(t, e, "default")
	checks, err := view.PlanChecks(e.App)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 3 || runs != 2 {
		t.Fatalf("checks %+v, %d runs", checks, runs)
	}
	for i, c := range checks {
		if pending := i == 2; c.Pending != pending || c.Stale {
			t.Fatalf("plan %d: %+v", i, c)
		}
	}
	// Enabled again, the edited plan has default's enabled set: it shares default's slot
	// (fresh or stale by its order), still without running anything.
	usePlan(t, e, "other")
	e.MustRun("enable", "pkg:tester.manual")
	other, _ := e.App.FindPlan("other")
	if c, err := view.PlanCheckOf(e.App, other); err != nil || c.Pending || runs != 2 {
		t.Fatalf("edited plan: %+v %v, %d runs", c, err, runs)
	}
}

// 08 §1, §4: the refusal and archo status name the plan running, from the launch records;
// a game started elsewhere plays the plan last written to the game.
func TestRunningGameNamesThePlan(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("launch")
	running := map[string]*app.GameRun{}
	e.App.GameProbe = func(root string) *app.GameRun { return running[env.PathKey(root)] }
	running[env.PathKey(e.Game)] = &app.GameRun{PID: 7, Exe: filepath.Join(e.Game, "RimWorldWin64.exe"), Started: e.Now().Add(20 * time.Second), ArgsKnown: true}
	if out, code := e.Run("launch"); code == 0 || !strings.Contains(out, "environment main is running plan:default (pid 7") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if out := e.MustRun("status"); !strings.Contains(out, "game: running plan:default for") {
		t.Fatal(out)
	}

	e.App.Store.DB.Exec(`DELETE FROM launch`)
	if out, _ := e.Run("launch"); !strings.Contains(out, "environment main is running plan:default (last written to the game) (pid 7") {
		t.Fatal(out)
	}
	e.App.Store.DB.Exec(`DELETE FROM modsconfig_state`)
	if out, _ := e.Run("launch"); !strings.Contains(out, "environment main is running its list (pid 7") {
		t.Fatal(out)
	}
}

// J13: after the game exits, a Player.log newer than the last launch and the last log the
// player looked at shows "analyse this run's log" (08 §2), until the log is looked at.
func TestNewLogHint(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var tr view.LiveTracker
	l, err := tr.Update(e.App, st)
	if err != nil || l.Running || l.NewLog != view.T("analyse this run's log") {
		t.Fatalf("new log: %+v %v", l, err)
	}
	cur, _ := e.App.LogPaths(st)
	if err := e.App.SelectLog(st, cur); err != nil {
		t.Fatal(err)
	}
	if l, _ := tr.Update(e.App, st); l.NewLog != "" {
		t.Fatalf("looked at: %+v", l)
	}
}
