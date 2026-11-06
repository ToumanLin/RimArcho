package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// probeEnv: a plan with two local Mods and a probe folder outside the game (09 §7.1).
func probeEnv(t *testing.T) (*testkit.Env, string) {
	t.Helper()
	e := testRunEnv(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "A"), testkit.Sample("test.a", "A"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), testkit.Sample("test.b", "B"))
	e.MustRun("scan")
	e.MustRun("enable", "pkg:test.a")
	e.MustRun("enable", "pkg:test.b")
	probe := filepath.Join(e.Root, "work", "probe")
	e.WriteMod(probe, testkit.Mod{PackageID: "Probe.Scenario", Name: "Probe", Author: "probe", Deps: []string{"test.a", "brrainz.harmony"}})
	return e, probe
}

const probeLog = "[Probe] startup scenario=Scenario order=test.a>probe.scenario\n" +
	"[Probe] setup\n" +
	"[Probe] t=250 a:Wait\n" +
	"[Probe] error in startup: Resolution too small (698x527), resetting UI scale to 1.\n" +
	"[Probe] RESULT smoke ok colonists=3 errors=[startup:0 setup:0]\n" +
	"[Probe] late line\n"

func TestTestRunProbe(t *testing.T) {
	t.Parallel()
	e, probe := probeEnv(t)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(e.Home, "tests", st.Ix.Env.ID)
	data := filepath.Join(base, "userdata")
	mods := filepath.Join(e.Game, "Mods")
	start := fakeTestGame(t, "probe", probeLog)
	var during []string
	var args []string
	var active []string
	e.App.TestGameStart = func(cmd *exec.Cmd) error {
		during = linkNames(t, mods)
		args = slices.Clone(cmd.Args)
		cfg, err := loadout.ReadModsConfig(filepath.Join(data, "Config", "ModsConfig.xml"))
		if err != nil {
			t.Error(err)
		} else {
			active = cfg.Active
		}
		if _, err := os.Stat(filepath.Join(data, "probe.png")); !os.IsNotExist(err) {
			t.Errorf("an earlier screenshot reached the launch: %v", err)
		}
		return start(cmd)
	}
	e.WriteFile(filepath.Join(data, "probe.png"), "stale")
	before := e.ReadModsConfig()
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{
		Probe: probe, ProbeAfter: "test.a", Timeout: 5 * time.Second, Settle: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	link := "_Archo_probe_probe.scenario"
	if !slices.Equal(during, []string{link}) || !slices.Equal(r.Linked, []string{link}) {
		t.Fatalf("links during the run %v, linked %v", during, r.Linked)
	}
	if left := linkNames(t, mods); len(left) != 0 {
		t.Fatalf("links left after the run: %v", left)
	}
	if !slices.Contains(args, "quicktest") || !slices.Contains(args, "-probequit") {
		t.Fatalf("args %q", args)
	}
	ia, ip, ib := slices.Index(active, "test.a"), slices.Index(active, "probe.scenario"), slices.Index(active, "test.b")
	if ia < 0 || ip != ia+1 || ib != ip+1 {
		t.Fatalf("test load order %v", active)
	}
	p := r.Probe
	if r.Status != "probe" || p == nil || p.Outcome != "result" || p.Result != "smoke ok colonists=3 errors=[startup:0 setup:0]" ||
		p.PackageID != "probe.scenario" || p.Position != ip+1 || p.LinesTotal != 6 || len(p.Lines) != 6 || p.Lines[2] != "t=250 a:Wait" {
		t.Fatalf("report %+v, probe %+v", r, p)
	}
	if !slices.Equal(p.MissingDeps, []string{"brrainz.harmony"}) {
		t.Fatalf("missing dependencies %v", p.MissingDeps)
	}
	if p.Screenshot != filepath.Join(r.RunDir, "probe.png") {
		t.Fatalf("screenshot %q", p.Screenshot)
	}
	if b, err := os.ReadFile(p.Screenshot); err != nil || string(b) != "png" {
		t.Fatalf("screenshot %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(data, "probe.png")); !os.IsNotExist(err) {
		t.Fatalf("screenshot left in the user data: %v", err)
	}
	for _, c := range r.Info {
		if strings.HasPrefix(c.Message, "[Probe] ") {
			t.Fatalf("probe line repeated as informational: %+v", c)
		}
	}
	if !r.Success() || r.Requested != len(st.List) {
		t.Fatalf("success %v, requested %d", r.Success(), r.Requested)
	}
	if e.ReadModsConfig() != before {
		t.Fatal("a probe run wrote the player's ModsConfig.xml")
	}
	after, err := e.App.Loadout()
	if err != nil || slices.ContainsFunc(after.List, func(en loadout.Entry) bool { return en.Missing == "probe.scenario" }) {
		t.Fatalf("plan changed: %v", err)
	}

	// The same probe again compares with the first run; a plain map run does not.
	e.App.TestGameStart = fakeTestGame(t, "probe", probeLog)
	again, err := e.App.TestRun(context.Background(), app.TestRunOptions{Probe: probe, Timeout: 5 * time.Second, Settle: time.Second})
	if err != nil || again.Baseline == nil || again.Baseline.RunDir != r.RunDir || again.Probe.Position != len(active) {
		t.Fatalf("second probe run %+v, %v", again, err)
	}
	e.App.TestGameStart = fakeTestGame(t, "idle", "Loading\nInitializing new game with mods\n")
	plain, err := e.App.TestRun(context.Background(), app.TestRunOptions{Map: true, Timeout: 700 * time.Millisecond, Settle: time.Millisecond})
	if err != nil || plain.Probe != nil || plain.Baseline != nil {
		t.Fatalf("plain map run %+v, %v", plain, err)
	}
}

func TestTestRunProbeOutcomes(t *testing.T) {
	t.Parallel()
	e, probe := probeEnv(t)
	var many strings.Builder
	for i := range 45 {
		fmt.Fprintf(&many, "[Probe] line %d %s\n", i, strings.Repeat("x", 400))
	}
	many.WriteString("[Probe] RESULT done\n")
	for _, tc := range []struct {
		name, mode, log, status, outcome string
		check                            func(*app.TestRunProbe) bool
	}{
		{"exception", "idle", "[Probe] setup\n[Probe] EXCEPTION in Setup System.Exception: boom\n  at RimProbe.Scenario.Setup (Verse.Map map) [0x0] in <x>:0\n  at RimProbe.ProbeMapComponent.MapComponentUpdate () [0x0] in <x>:0\nUnityEngine.StackTraceUtility:ExtractStackTrace ()\n",
			"probe", "exception", func(p *app.TestRunProbe) bool {
				return p.Exception == "in Setup System.Exception: boom\nat RimProbe.Scenario.Setup (Verse.Map map) [0x0] in <x>:0\nat RimProbe.ProbeMapComponent.MapComponentUpdate () [0x0] in <x>:0"
			}},
		{"no-line", "exit", "[Probe] startup\n", "exited", "none", func(p *app.TestRunProbe) bool { return p.LinesTotal == 1 }},
		{"many", "probe", many.String(), "probe", "result", func(p *app.TestRunProbe) bool {
			return p.LinesTotal == 46 && len(p.Lines) == 40 && strings.HasPrefix(p.Lines[9], "line 9 ") &&
				strings.HasPrefix(p.Lines[10], "line 16 ") && p.Lines[39] == "RESULT done" && len([]rune(p.Lines[0])) == 300
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e.App.TestGameStart = fakeTestGame(t, tc.mode, tc.log)
			r, err := e.App.TestRun(context.Background(), app.TestRunOptions{Probe: probe, ProbeBefore: "test.a", Timeout: 5 * time.Second, Settle: 300 * time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			// Only a RESULT line can succeed; this log has no error clusters.
			if r.Status != tc.status || r.Probe.Outcome != tc.outcome || !tc.check(r.Probe) || r.Success() != (tc.outcome == "result") {
				t.Fatalf("report %+v, probe %+v", r, r.Probe)
			}
			if r.Probe.Position != 2 { // before test.a, after Core
				t.Fatalf("position %d", r.Probe.Position)
			}
			if r.Probe.Screenshot != "" && tc.mode != "probe" {
				t.Fatalf("screenshot %q without one", r.Probe.Screenshot)
			}
		})
	}
}

// Every probe check fails before anything is linked or started (09 §7.1).
func TestTestRunProbeChecks(t *testing.T) {
	t.Parallel()
	e, probe := probeEnv(t)
	noAbout := filepath.Join(e.Root, "work", "empty")
	e.MkdirAll(noAbout)
	official := filepath.Join(e.Root, "work", "official")
	e.WriteMod(official, testkit.Mod{PackageID: "ludeon.rimworld.probe", Name: "Official"})
	listed := filepath.Join(e.Root, "work", "listed")
	e.WriteMod(listed, testkit.Mod{PackageID: "test.b", Name: "Listed"})
	for _, tc := range []struct {
		name string
		o    app.TestRunOptions
		want string
	}{
		{"missing", app.TestRunOptions{Probe: filepath.Join(e.Root, "nowhere")}, "not found"},
		{"no-about", app.TestRunOptions{Probe: noAbout}, "no About/About.xml"},
		{"official", app.TestRunOptions{Probe: official}, "official content"},
		{"listed", app.TestRunOptions{Probe: listed}, "already lists test.b"},
		{"after-unknown", app.TestRunOptions{Probe: probe, ProbeAfter: "test.zzz"}, "no test.zzz"},
		{"both", app.TestRunOptions{Probe: probe, ProbeAfter: "test.a", ProbeBefore: "test.b"}, "not both"},
		{"position-only", app.TestRunOptions{ProbeAfter: "test.a"}, "without a probe folder"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e.App.TestGameStart = func(*exec.Cmd) error { t.Error("started"); return fmt.Errorf("must not start") }
			r, err := e.App.TestRun(context.Background(), tc.o)
			if err == nil || r != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%+v, %v", r, err)
			}
			if left := linkNames(t, filepath.Join(e.Game, "Mods")); len(left) != 0 {
				t.Fatalf("links: %v", left)
			}
		})
	}
	// Another Mods/ copy of the probe's packageId: the game would load only one.
	e.WriteMod(filepath.Join(e.Game, "Mods", "ProbeCopy"), testkit.Mod{PackageID: "probe.scenario", Name: "Copy"})
	e.App.TestGameStart = func(*exec.Cmd) error { t.Error("started"); return fmt.Errorf("must not start") }
	if _, err := e.App.TestRun(context.Background(), app.TestRunOptions{Probe: probe}); err == nil || !strings.Contains(err.Error(), "Mods/ProbeCopy") {
		t.Fatalf("same packageId in Mods/: %v", err)
	}
	// A probe that is in Mods/ already is used where it is, without a link.
	e.App.TestGameStart = fakeTestGame(t, "probe", probeLog)
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{Probe: filepath.Join(e.Game, "Mods", "ProbeCopy"), Timeout: 5 * time.Second, Settle: time.Second})
	if err != nil || len(r.Linked) != 0 || r.Probe.Outcome != "result" {
		t.Fatalf("probe in Mods/: %+v, %v", r, err)
	}
}

func TestTestRunProbeCLI(t *testing.T) {
	t.Parallel()
	e, probe := probeEnv(t)
	e.App.TestGameStart = fakeTestGame(t, "probe", probeLog)
	out, code := e.Run("test", "run", "--probe", probe, "--probe-before", "test.b", "--timeout", "5s", "--settle", "1s")
	for _, want := range []string{"status: probe", "probe: probe.scenario, load order position", "outcome: result",
		"result: smoke ok colonists=3", "missing dependencies: brrainz.harmony", "screenshot: ", "[Probe] lines: 6", "t=250 a:Wait"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	out, code = e.Run("test", "run", "--probe", probe, "--timeout", "5s", "--settle", "1s", "--json")
	var r app.TestRunReport
	if code != 0 || json.Unmarshal([]byte(out), &r) != nil || r.Probe == nil || r.Probe.Outcome != "result" || r.Args != nil {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, flags := range [][]string{{"--probe-after", "test.a"}, {"--probe", probe, "--probe-after", "test.a", "--probe-before", "test.b"}} {
		out, code := e.Run(append([]string{"test", "run"}, flags...)...)
		if code != cmds.ExitUsage || !strings.HasPrefix(out, "error: ") {
			t.Fatalf("bad flags %v: %d, %s", flags, code, out)
		}
	}
}
