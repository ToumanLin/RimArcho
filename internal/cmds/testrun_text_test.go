package cmds

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// testRunReports are synthetic test run reports that reach every branch of the text report.
func testRunReports() map[string]*app.TestRunReport {
	cl := func(class, msg string, n int) app.TestRunCluster {
		return app.TestRunCluster{Class: class, Message: msg, Count: n, Mods: []string{"#3 Alpha"}, Confidence: "high"}
	}
	full := &app.TestRunReport{
		Env: "main", Plan: "p:2 Colony", GameVersion: "1.6.4633", Status: "map", NotTested: "the run stopped before its target",
		LoadSeconds: 41.25, TotalSeconds: 63.5, TimeoutSeconds: 600, PeakMemoryMB: 4096, Requested: 12,
		Save:       &app.TestRunSave{Name: "Autosave-1", Path: `C:\Saves\Autosave-1.rws`, Added: []string{"a.b", "c.d"}, OrderDiffers: true},
		MapSeconds: 12.5, TicksReached: 4000,
		Linked:      []string{"Alpha", "Beta"},
		Hidden:      []string{"Alpha_old"},
		Loaded:      []app.TestRunLoaded{{PackageID: "a.b", Folder: "Mods/Alpha", From: "log"}},
		LoadedTotal: 3,
		Missing:     []app.TestRunMissing{{PackageID: "x.y", Name: "Gamma", Reason: "not installed"}, {PackageID: "z.z", Name: "Delta"}},
		Clusters: []app.TestRunCluster{
			func() app.TestRunCluster {
				c := cl("exception", "boom", 3)
				c.Delta, c.Before, c.Note = "new", "present", "common kind"
				return c
			}(),
			func() app.TestRunCluster {
				c := cl("xml", "bad def", 1)
				c.Before, c.Consequences = "absent", 2
				return c
			}(),
		},
		ClustersTotal: 7, LogPath: `C:\run\Player.log`,
		Info: []app.TestRunCluster{
			func() app.TestRunCluster { c := cl("modreport", "hello", 2); c.Before = "present"; return c }(),
			func() app.TestRunCluster { c := cl("other", "noise", 1); c.Before = "absent"; return c }(),
			cl("other", "plain", 1),
		},
		PlayerLog:   &app.TestRunPlayerLog{Path: `C:\Player.log`, Present: 1, Absent: 1, Fixed: 1, Unreached: 2},
		Fixed:       []app.TestRunCluster{func() app.TestRunCluster { c := cl("exception", "gone now", 4); c.Note = "a note"; return c }()},
		Baseline:    &app.TestRunBaseline{RunDir: `C:/runs/20261121-1`, Status: "map", New: 1, Present: 1, Gone: 1, Unreached: 3},
		Gone:        []app.TestRunCluster{cl("exception", "old", 1)},
		Attribution: "fresh analysis", RunDir: `C:\runs\20261122-1`,
	}
	plain := &app.TestRunReport{Env: "main", Plan: "p:1 Default", GameVersion: "1.6.4633", Status: "menu", TimeoutSeconds: 600,
		RunDir: `C:\runs\1`, Attribution: "none", MapSeconds: 3, TicksEstimated: 180}
	logErr := &app.TestRunReport{Env: "e", Plan: "p", Status: "exited", PlayerLog: &app.TestRunPlayerLog{Path: `C:\Player.log`, Error: "unreadable"}}
	return map[string]*app.TestRunReport{"full": full, "plain": plain, "player log error": logErr}
}

// testRunTextWant is the report of each of testRunReports, as before the report was split
// into steps.
var testRunTextWant = map[string]string{
	"full": `test run: p:2 Colony  env:main
  NOT TESTED: the run stopped before its target
  status: map
  game: 1.6.4633
  load: 41.2s (total 63.5s, limit 600s)
  save: Autosave-1 (C:\Saves\Autosave-1.rws)
    not in the save: a.b, c.d (added to the colony)
    order: the plan loads the save's Mods in another order
  map: 4000 ticks played (first autosave), 12.5s after the save load
  peak memory: 4096 MB
  requested: 12
  linked for this run (removed after it): 2:
    Mods/Alpha
    Mods/Beta
  moved aside for this run (same packageId as a linked Mod; back after it): 1:
    Mods/Alpha_old
  loaded from: 1 of 3:
    a.b  Mods/Alpha  (log)
  missing: 2:
    x.y  Gamma
      not loaded: not installed
    z.z  Delta
  error clusters: 7:
    new exception x3: boom
      before: in the player's log
      mods: #3 Alpha (high)
      note: common kind
    xml x1: bad def
      before: not in the player's log
      consequences: 2 clusters that follow from this def file failing to load, not listed
      mods: #3 Alpha (high)
    … 3 more (full log: C:\run\Player.log)
  informational: 3:
    modreport x2: hello  (in the player's log)
    other x1: noise  (not in the player's log)
    other x1: plain
  compared with the player's log C:\Player.log: 1 present, 1 absent, 1 fixed, 2 not reached by this run
  fixed: 1:
    exception x4: gone now
      mods: #3 Alpha (high)
      note: a note
  compared with run 20261121-1 (map): 1 new, 1 present, 1 gone, 3 not reached by this run
  gone: 1:
    exception x1: old
      mods: #3 Alpha (high)
  attribution: fresh analysis
  run dir: C:\runs\20261122-1
`,
	"plain": `test run: p:1 Default  env:main
  status: menu
  game: 1.6.4633
  load: 0.0s (total 0.0s, limit 600s)
  map: ran 3.0s after the new-game marker (at most ~180 ticks at normal speed; map generation counts in that time)
  peak memory: 0 MB
  requested: 0
  missing: 0:
  error clusters: 0:
  compared with the player's log: none found
  compared with: no earlier run of this plan
  attribution: none
  run dir: C:\runs\1
`,
	"player log error": `test run: p  env:e
  status: exited
  load: 0.0s (total 0.0s, limit 0s)
  peak memory: 0 MB
  requested: 0
  missing: 0:
  error clusters: 0:
  compared with the player's log C:\Player.log: unreadable
  compared with: no earlier run of this plan
`,
}

// TestTestRunText pins the text report of archo test run.
func TestTestRunText(t *testing.T) {
	t.Parallel()
	for name, r := range testRunReports() {
		if got, want := testRunDoc(r).String(), testRunTextWant[name]; got != want {
			t.Errorf("%s:\ngot\n%s\nwant\n%s", name, got, want)
		}
	}
}

// TestTestRunOptions checks the flag errors of archo test run, which stop before the game starts.
func TestTestRunOptions(t *testing.T) {
	t.Parallel()
	command, _ := lookup([]string{"test", "run"})
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--ticks", "0"}, "--ticks needs a positive number"},
		{[]string{"--ticks", "x"}, "--ticks needs a positive number"},
		{[]string{"--save", "s", "--map"}, "--save loads a save"},
		{[]string{"--save", "s", "--probe", "p"}, "--save loads a save"},
		{[]string{"--ticks", "10"}, "--ticks needs --map or --save"},
		{[]string{"--ticks", "10", "--map", "--probe", "p"}, "--ticks needs --map or --save"},
		{[]string{"--probe-after", "a"}, "--probe-after and --probe-before need --probe"},
		{[]string{"--probe", "p", "--probe-after", "a", "--probe-before", "b"}, "give --probe-after or --probe-before, not both"},
		{[]string{"--timeout", "0s"}, "--timeout needs a positive duration"},
		{[]string{"--settle", "soon"}, "--settle needs a positive duration"},
		{[]string{"--map", "--ticks", "100", "--timeout", "5m", "--settle", "10s", "p:2"}, ""},
	} {
		a, err := parse(command, tc.args)
		if err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		o, err := testRunOptions(&Ctx{App: &app.App{}}, a)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%v: %v", tc.args, err)
		case tc.want == "" && (o.Plan != "p:2" || o.Ticks != 100 || !o.Map || o.Timeout.String() != "5m0s" || o.Settle.String() != "10s"):
			t.Errorf("%v: %+v", tc.args, o)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%v: got %v, want %q", tc.args, err, tc.want)
		}
	}
}
