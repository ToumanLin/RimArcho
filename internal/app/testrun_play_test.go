package app_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// playEnv is a fixture whose plan has Core and one Workshop Mod, Played.
func playEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := testRunEnv(t)
	e.AddWorkshop("2001", testkit.Sample("test.played", "Played"))
	e.MustRun("scan")
	e.MustRun("enable", "pkg:test.played")
	return e
}

// h8 B1: the player's log loaded a save; a menu run cannot show what happened after the
// load, so that error is not reached, not fixed. The error before the load is fixed.
func TestTestRunGameMarksNotFixed(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	writePlayerLog(e, comparisonLog("Early")+"Loading game from file autostart with mods:\n  - ludeon.rimworld\n\n"+comparisonLog("AfterLoad"))
	r := testRunWith(t, e, "menu", "Loading\n")
	if p := r.PlayerLog; p == nil || p.Fixed != 1 || p.Unreached != 1 || len(r.Fixed) != 1 || !strings.Contains(r.Fixed[0].Message, "Early") {
		t.Fatalf("player log %+v, fixed %+v", r.PlayerLog, r.Fixed)
	}
	if r.Phase != "menu" || r.Status != "menu" || r.NotTested != "" || r.TimeoutSeconds != 5 {
		t.Fatalf("report %+v", r)
	}
}

// h8 B2 (c) and (b): a plan Mod the game's running list lacks was not loaded: the run is
// incomplete and says so, and the player's errors count as not reached. Ticks end a map
// run at the first autosave and report how long the map ran (h7 G6).
func TestTestRunIncompleteAndTicks(t *testing.T) {
	t.Parallel()
	e := playEnv(t)
	writePlayerLog(e, comparisonLog("Gone"))
	e.App.TestGameStart = fakeTestGame(t, "autosave", "Initializing new game with mods:\n  - ludeon.rimworld\n\n")
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{Map: true, Ticks: 600, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "incomplete" || r.Success() || !strings.Contains(r.NotTested, "these Mods were not loaded: Played (test.played)") ||
		len(r.Missing) != 1 || !strings.Contains(r.Missing[0].Reason, "running Mods") {
		t.Fatalf("report %+v", r)
	}
	if r.Ticks != 600 || r.TicksReached != 600 || r.TicksEstimated != 600 || r.Phase != "new" || r.MapSeconds <= 0 {
		t.Fatalf("ticks %+v", r)
	}
	if p := r.PlayerLog; p == nil || p.Fixed != 0 || p.Unreached != 1 || len(r.Fixed) != 0 {
		t.Fatalf("an incomplete run fixed something: %+v", r.PlayerLog)
	}
	out, code := e.Run("test", "run", "--map", "--ticks", "600", "--timeout", "20s")
	for _, want := range []string{"NOT TESTED: these Mods were not loaded: Played (test.played)", "status: incomplete", "map: 600 ticks played"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	// With the Mod in the running list the same run is a test.
	e.App.TestGameStart = fakeTestGame(t, "autosave", "Initializing new game with mods:\n  - ludeon.rimworld\n  - test.played\n\n")
	r, err = e.App.TestRun(context.Background(), app.TestRunOptions{Map: true, Ticks: 600, Timeout: 20 * time.Second})
	if err != nil || r.Status != "map" || r.NotTested != "" || !r.Success() || r.LoadedTotal != 1 || r.Loaded[0].Folder != "Workshop/2001" {
		t.Fatalf("report %+v, %v", r, err)
	}
}

// h8 G2: a run that times out says it tested nothing and gives the log's last line; a map
// run without a limit gets twice the previous map run's load time, at least 10 minutes.
func TestTestRunTimeoutNotTested(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	e.App.TestGameStart = fakeTestGame(t, "idle", "Loading\nInitializing new game with mods:\n  - ludeon.rimworld\n\nStill generating the map\n")
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{Map: true, Ticks: 600, Timeout: 700 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "timeout" || r.LastLogLine != "Still generating the map" ||
		!strings.HasPrefix(r.NotTested, "the game did not reach 600 ticks on the map within 1 s (it got to a new game); last log line: Still generating the map") {
		t.Fatalf("report %+v", r)
	}
	out, _ := e.Run("test", "run", "--map", "--ticks", "600", "--timeout", "700ms")
	if !strings.Contains(out, "NOT TESTED: the game did not reach 600 ticks on the map") {
		t.Fatalf("text:\n%s", out)
	}
	// A previous map run that arrived after 400 s sets the next default limit.
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	runs := filepath.Join(e.Home, "tests", st.Ix.Env.ID, "runs")
	old := filepath.Join(runs, "20000101-000000")
	e.WriteFile(filepath.Join(old, "report.json"), `{"status":"map","args":["quicktest"],"load_seconds":400,"ticks":600}`)
	e.App.TestGameStart = func(*exec.Cmd) error { return os.ErrPermission }
	for _, c := range []struct {
		o    app.TestRunOptions
		want float64
	}{
		{app.TestRunOptions{Map: true, Ticks: 600}, 800},
		{app.TestRunOptions{Map: true, Ticks: 6600}, 1000}, // 6,000 more ticks: 100 s at normal speed, twice
		{app.TestRunOptions{}, 600},
	} {
		r, _ := e.App.TestRun(context.Background(), c.o)
		if r == nil || r.TimeoutSeconds != c.want {
			t.Fatalf("%+v: timeout %+v", c.o, r)
		}
	}
}

// h8 G1: a save run loads the player's save in the run's own data and plays ticks; a save
// with Mods the plan lacks is refused.
func TestTestRunSave(t *testing.T) {
	t.Parallel()
	e := playEnv(t)
	path := e.WriteSave("Colony.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "test.played"}})
	writePlayerLog(e, "Loading game from file Colony with mods:\n  - ludeon.rimworld\n  - test.played\n\n"+comparisonLog("InPlay"))
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(e.Home, "tests", st.Ix.Env.ID, "userdata")
	var prefs, autostart []byte
	var args []string
	start := fakeTestGame(t, "autosave", "Loading game from file autostart with mods:\n  - ludeon.rimworld\n  - test.played\n\n")
	e.App.TestGameStart = func(cmd *exec.Cmd) error {
		prefs, _ = os.ReadFile(filepath.Join(data, "Config", "Prefs.xml"))
		autostart, _ = os.ReadFile(filepath.Join(data, "Saves", "autostart.rws"))
		args = cmd.Args
		return start(cmd)
	}
	before, _ := os.ReadFile(path)
	out, code := e.Run("test", "run", "--save", "Colony", "--timeout", "20s", "--json")
	var r app.TestRunReport
	if err := json.Unmarshal([]byte(out), &r); err != nil || code != 0 {
		t.Fatalf("exit %d: %v\n%s", code, err, out)
	}
	if r.Status != "save" || r.Phase != "save" || r.Ticks != 4000 || r.TicksReached != 4000 || r.Save == nil || r.Save.Name != "Colony" || r.Save.Path != path {
		t.Fatalf("report %+v", r)
	}
	if !strings.Contains(string(prefs), "<devMode>True</devMode>") || string(autostart) != string(before) || slices.Contains(args, "quicktest") {
		t.Fatalf("prefs %s, autostart %d bytes, args %q", prefs, len(autostart), args)
	}
	// The player's error after loading this save is reached: gone now, so fixed.
	if p := r.PlayerLog; p == nil || p.Fixed != 1 || p.Unreached != 0 {
		t.Fatalf("player log %+v", r.PlayerLog)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("the player's save changed")
	}
	// The plan has a Mod the save does not: allowed and named. The save has one the plan
	// does not: refused with the Mods named.
	e.WriteSave("Fewer.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}})
	e.WriteSave("More.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "test.played", "other.mod"}, Names: []string{"", "", "Other Mod"}})
	e.WriteSave("Old.rws", testkit.SaveSpec{Game: "1.5.4409 rev1118", Mods: []string{"ludeon.rimworld", "test.played"}})
	e.WriteSave("Cut.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, CutOff: true})
	r2, err := e.App.TestRun(context.Background(), app.TestRunOptions{Save: "Fewer", Timeout: 20 * time.Second})
	if err != nil || r2.Save == nil || !slices.Equal(r2.Save.Added, []string{"Played"}) {
		t.Fatalf("fewer: %+v, %v", r2, err)
	}
	for save, want := range map[string]string{
		"More": "uses 1 Mods the plan does not have: Other Mod (other.mod)", "Old": "from RimWorld 1.5",
		"Cut": "cut off", "Missing": "no such save",
	} {
		if _, err := e.App.TestRun(context.Background(), app.TestRunOptions{Save: save}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", save, err, want)
		}
	}
	for _, flags := range [][]string{{"--save", "Colony", "--map"}, {"--ticks", "100"}, {"--ticks", "-1", "--map"}} {
		if _, code := e.Run(append([]string{"test", "run"}, flags...)...); code == 0 {
			t.Errorf("%v accepted", flags)
		}
	}
}

// A run interrupted while a Mods/ entry was moved aside: the next run moves it back first;
// when a new entry has taken its name, it stays aside and the run says where (20 §2).
func TestTestRunHiddenRestore(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(e.Home, "tests", st.Ix.Env.ID)
	mods := filepath.Join(e.Game, "Mods")
	aside := filepath.Join(e.Game, "_ArchoTestHidden")
	e.WriteMod(filepath.Join(aside, "Back"), testkit.Sample("test.back", "Back"))
	e.WriteMod(filepath.Join(aside, "Taken"), testkit.Sample("test.taken", "Taken"))
	e.WriteMod(filepath.Join(mods, "Taken"), testkit.Sample("test.new", "New"))
	rec, _ := json.Marshal(map[string]any{"mods": mods, "hidden": []map[string]string{
		{"name": "Back", "to": filepath.Join(aside, "Back")}, {"name": "Taken", "to": filepath.Join(aside, "Taken")},
	}})
	e.WriteFile(filepath.Join(base, "links.json"), string(rec))
	if _, err := e.App.TestRun(context.Background(), app.TestRunOptions{Timeout: 5 * time.Second, Settle: time.Millisecond}); err == nil ||
		!strings.Contains(err.Error(), "Mods/Taken exists again") {
		t.Fatalf("error %v", err)
	}
	if _, err := os.Stat(filepath.Join(mods, "Back", "About", "About.xml")); err != nil {
		t.Fatalf("Back not moved back: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(base, "links.json"))
	if !strings.Contains(string(b), "Taken") || strings.Contains(string(b), `"Back"`) {
		t.Fatalf("record %s", b)
	}
}

// Dev mode's reference check in the run's own autosave gets a note, and stays an error (06 §1).
func TestTestRunDeepSavedNote(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	e.App.TestGameStart = fakeTestGame(t, "autosave", "Initializing new game with mods:\n  - ludeon.rimworld\n\n"+
		"Object with load ID Thing_Human3842 is referenced (xml node name: li) but is not deep-saved. This will cause errors during loading.\n")
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{Map: true, Ticks: 600, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "map" || r.Counts[logs.ClassSave] != 1 || r.Success() || len(r.Clusters) != 1 || !strings.HasPrefix(r.Clusters[0].Note, "written while this run's autosave") {
		t.Fatalf("report %+v", r)
	}
}
