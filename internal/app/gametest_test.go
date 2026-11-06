package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/gametest"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func testRunEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	e.AddEnv(true)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	e.MustRun("scan")
	e.App.TestGameStart = fakeTestGame(t, "menu", "")
	return e
}

func fakeTestGame(t *testing.T, mode, text string) func(*exec.Cmd) error {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return func(cmd *exec.Cmd) error {
		var data, log string
		for i, arg := range cmd.Args {
			if strings.HasPrefix(arg, "-savedatafolder=") {
				data = strings.TrimPrefix(arg, "-savedatafolder=")
			}
			if arg == "-logfile" {
				log = cmd.Args[i+1]
			}
		}
		if data == "" || cmd.Dir == "" {
			return fmt.Errorf("missing data directory or working directory")
		}
		cmd.Env = append(os.Environ(), "ARCHO_TEST_GAME="+mode, "ARCHO_TEST_DATA="+data, "ARCHO_TEST_LOG="+log, "ARCHO_TEST_TEXT="+text)
		cmd.Path = exe
		cmd.Args = []string{exe, "-test.run=^TestAppGameHelper$"}
		return cmd.Start()
	}
}

func TestAppGameHelper(t *testing.T) {
	t.Parallel()
	mode := os.Getenv("ARCHO_TEST_GAME")
	if mode == "" {
		return
	}
	log := os.Getenv("ARCHO_TEST_LOG")
	text := os.Getenv("ARCHO_TEST_TEXT")
	if log != "" {
		if err := os.WriteFile(log, []byte("RimWorld 1.6.4633 rev1261\n"+text), 0o644); err != nil {
			os.Exit(98)
		}
	} else {
		fmt.Print(text)
	}
	if mode == "menu" {
		time.Sleep(20 * time.Millisecond)
		if err := os.WriteFile(filepath.Join(os.Getenv("ARCHO_TEST_DATA"), "Config", "Knowledge.xml"), []byte("menu"), 0o644); err != nil {
			os.Exit(97)
		}
	} else if mode == "exit" {
		os.Exit(7)
	} else if mode == "probe" { // a probe saves its screenshot, logs its RESULT and quits (09 §7.1)
		if err := os.WriteFile(filepath.Join(os.Getenv("ARCHO_TEST_DATA"), "probe.png"), []byte("png"), 0o644); err != nil {
			os.Exit(96)
		}
		os.Exit(0)
	}
	time.Sleep(30 * time.Second)
}

func TestTestRunPlanConfigVisibilityAndReport(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	e.AddWorkshop("1001", testkit.Sample("test.only", "OnlyTest"))
	e.WriteMod(filepath.Join(e.SteamCMD, "1002"), testkit.Sample("test.cmd", "CmdOnly"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Loose"), testkit.Sample("test.local", "Loose"))
	e.WriteMod(filepath.Join(e.Game, "Data", "Royalty"), testkit.Sample("ludeon.rimworld.royalty", "Royalty"))
	extra := filepath.Join(e.Root, "extra")
	e.WriteMod(filepath.Join(extra, "Linked"), testkit.Sample("test.linked", "Linked"))
	e.WriteMod(filepath.Join(extra, "ExtraOnly"), testkit.Sample("test.extra", "ExtraOnly"))
	e.MustRun("folder", "add", "extra", extra)
	e.MustRun("scan")
	for _, ref := range []string{"test.only", "test.cmd", "test.local", "test.linked", "test.extra", "ludeon.rimworld.royalty"} {
		e.MustRun("enable", "pkg:"+ref)
	}
	e.MustRun("plan", "new", "tested", "--from", "current")
	e.MustRun("disable", "pkg:test.only")
	e.MustRun("env", "set", "main", "--launch", "steam")
	e.Junction(filepath.Join(e.Game, "Mods", "ExistingLink"), filepath.Join(extra, "Linked"))
	prefs, err := os.ReadFile("../gametest/testdata/Prefs.xml")
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(e.UserData, "Config", "Prefs.xml"), string(prefs))
	e.WriteFile(filepath.Join(e.UserData, "Config", "KeyPrefs.xml"), "<KeyPrefs/>")
	e.WriteFile(filepath.Join(e.UserData, "Config", "ModSettings.xml"), "<Settings>keep</Settings>")
	// Linked is seen through another tool's link now; its settings were saved under its
	// RimArcho link name (09 §3).
	e.WriteFile(filepath.Join(e.UserData, "Config", "Mod__Archo_extra_Linked_LinkedMod.xml"), "<linked/>")
	e.WriteFile(filepath.Join(e.UserData, "Config", "Knowledge.xml"), "player knowledge")
	e.WriteFile(filepath.Join(e.UserData, "Config", "skip.txt"), "not config")
	e.WriteFile(filepath.Join(e.UserData, "Logs", "Player.log"), "player log")
	before := e.ReadModsConfig()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	beforePlan := st.Plan.ID
	base := filepath.Join(e.Home, "tests", st.Ix.Env.ID)
	data := filepath.Join(base, "userdata")
	e.WriteFile(filepath.Join(data, "Config", "Prefs.xml"), "old settings")
	e.WriteFile(filepath.Join(data, "ShaderCache", "keep"), "cached")
	for i := range 12 {
		e.WriteFile(filepath.Join(base, "runs", testkit.Start.Add(-time.Duration(i+1)*time.Second).Format("20060102-150405"), "result.json"), "{}")
	}
	e.WriteFile(filepath.Join(base, "runs", "keep-other", "keep"), "owned elsewhere")
	// Attribution must find OnlyTest even though it is disabled in the current plan.
	e.App.TestGameStart = fakeTestGame(t, "menu", "[OnlyTest] Exception: test failure\n  at OnlyTest.Mod.Start()\n")
	start := e.App.TestGameStart
	var linkedDuringRun []string
	e.App.TestGameStart = func(cmd *exec.Cmd) error {
		linkedDuringRun = linkNames(t, filepath.Join(e.Game, "Mods"))
		for _, name := range []string{"Prefs.xml", "KeyPrefs.xml"} {
			if _, err := os.Lstat(filepath.Join(data, "Config", name)); !os.IsNotExist(err) {
				t.Errorf("player preferences reached test launch: %s: %v", name, err)
			}
		}
		return start(cmd)
	}
	var progress []string
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{
		Env: "main", Plan: "tested", Timeout: 5 * time.Second, Settle: time.Millisecond,
		Progress: func(s string) { progress = append(progress, s) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "menu" || r.Plan != "plan:tested" || r.Requested != 7 || r.LoadSeconds <= 0 || r.TotalSeconds <= r.LoadSeconds || r.Attribution != "tested-plan" {
		t.Fatalf("report %+v", r)
	}
	// The SteamCMD and extra Mods are linked for the run only (09 §7).
	if len(r.Missing) != 0 || len(r.Linked) != 2 || !slices.Equal(linkedDuringRun, r.Linked) {
		t.Fatalf("missing %+v, linked %v, links during the run %v", r.Missing, r.Linked, linkedDuringRun)
	}
	if left := linkNames(t, filepath.Join(e.Game, "Mods")); len(left) != 0 {
		t.Fatalf("links left after the run: %v", left)
	}
	if _, err := os.Stat(filepath.Join(base, "links.json")); !os.IsNotExist(err) {
		t.Fatalf("link record left: %v", err)
	}
	if r.ClustersTotal != 1 || r.Counts[logs.ClassException] != 1 || len(r.Clusters) != 1 ||
		!slices.Contains(r.Clusters[0].Mods, "OnlyTest") || r.Clusters[0].Confidence == "" || r.Success() {
		t.Fatalf("clusters %+v, counts %v", r.Clusters, r.Counts)
	}
	config, err := loadout.ReadModsConfig(filepath.Join(data, "Config", "ModsConfig.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != st.Ix.Env.GameVersion || !slices.Contains(config.Active, "test.only") || !slices.Equal(config.Known, []string{"ludeon.rimworld.royalty"}) {
		t.Fatalf("test config %+v", config)
	}
	for path, want := range map[string]string{
		filepath.Join(e.UserData, "Config", "Prefs.xml"):                string(prefs),
		filepath.Join(e.UserData, "Config", "KeyPrefs.xml"):             "<KeyPrefs/>",
		filepath.Join(data, "Config", "ModSettings.xml"):                "<Settings>keep</Settings>",
		filepath.Join(data, "Config", "Mod_ExistingLink_LinkedMod.xml"): "<linked/>",
		filepath.Join(data, "ShaderCache", "keep"):                      "cached",
		filepath.Join(e.UserData, "Config", "Knowledge.xml"):            "player knowledge",
		filepath.Join(e.UserData, "Logs", "Player.log"):                 "player log",
	} {
		b, err := os.ReadFile(path)
		if err != nil || string(b) != want {
			t.Errorf("%s: %q, %v", path, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(data, "Config", "skip.txt")); !os.IsNotExist(err) {
		t.Fatal("copied non-XML settings")
	}
	if _, err := os.Stat(filepath.Join(e.UserData, "Config", "Mod_ExistingLink_LinkedMod.xml")); !os.IsNotExist(err) {
		t.Fatal("a test run wrote the player's Config")
	}
	if len(progress) < 3 || len(e.Launched) != 0 || e.ReadModsConfig() != before {
		t.Fatalf("progress %v; ordinary launches %v; player config changed", progress, e.Launched)
	}
	after, err := e.App.Loadout()
	if err != nil || after.Plan.ID != beforePlan || after.List.Index(st.List[0].Instance) < 0 || e.App.EnvName != "" {
		t.Fatalf("current plan/environment changed: %+v, %v", after, err)
	}
	b, err := os.ReadFile(filepath.Join(r.RunDir, "report.json"))
	wantJSON, marshalErr := json.MarshalIndent(r, "", "  ")
	if err != nil || marshalErr != nil || string(b) != string(append(wantJSON, '\n')) {
		t.Fatalf("saved report %s: %v", b, err)
	}
	dirs, err := os.ReadDir(filepath.Join(base, "runs"))
	if err != nil || len(dirs) != 11 { // Ten runs and the unrelated directory.
		t.Fatalf("retained %d entries: %v", len(dirs), err)
	}
	if _, err := os.Stat(filepath.Join(base, "runs", "keep-other", "keep")); err != nil {
		t.Fatal("removed unrelated directory")
	}
}

func TestTestRunGuardAndRunningGame(t *testing.T) {
	for _, kind := range []string{"game", "player", "default", "config-link", "running"} {
		t.Run(kind, func(t *testing.T) {
			e := testRunEnv(t)
			st, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(e.Home, "tests", st.Ix.Env.ID)
			switch kind {
			case "game":
				e.Junction(filepath.Join(e.Home, "tests"), e.Game)
			case "player":
				e.Junction(filepath.Join(e.Home, "tests"), e.UserData)
			case "default":
				// Simulate the platform default inside this fixture.
				if runtime.GOOS != "windows" {
					t.Skip("Windows default data directory")
				}
				t.Setenv("USERPROFILE", e.Root)
				e.MustRun("env", "set", "main", "--user-data", filepath.Join(e.Root, "other-userdata"))
				e.App.Home = filepath.Join(e.UserData, "ArchoHome")
			case "config-link":
				e.Junction(filepath.Join(base, "userdata", "Config"), filepath.Join(e.UserData, "Config"))
			case "running":
				e.App.GameProbe = func(string) *app.GameRun {
					return &app.GameRun{PID: 42, Exe: filepath.Join(e.Game, "RimWorldWin64.exe")}
				}
			}
			called := false
			e.App.TestGameStart = func(*exec.Cmd) error { called = true; return errors.New("must not start") }
			before := e.ReadModsConfig()
			if r, err := e.App.TestRun(context.Background(), app.TestRunOptions{}); err == nil || r != nil || called {
				t.Fatalf("guard: %+v, %v; started %v", r, err, called)
			}
			if e.ReadModsConfig() != before {
				t.Fatal("player config changed")
			}
		})
	}
}

func TestTestRunCLIOutcomes(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	for _, tc := range []struct {
		name, mode, log, status string
		code                    int
	}{
		{"clean", "menu", "Loading\n", "menu", 0},
		{"errors", "menu", "Exception: bad mod\n  at Bad.Mod.Start()\n", "menu", 1},
		{"exited", "exit", "Loading\n", "exited", 1},
		{"timeout", "idle", "Loading\n", "timeout", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e.App.TestGameStart = fakeTestGame(t, tc.mode, tc.log)
			timeout := "5s"
			if tc.mode == "idle" {
				timeout = "600ms"
			}
			out, code := e.Run("test", "run", "--env", "main", "--timeout", timeout, "--settle", "1ms", "--json")
			var r app.TestRunReport
			if code != tc.code || json.Unmarshal([]byte(out), &r) != nil || r.Status != tc.status {
				t.Fatalf("exit %d, output %s", code, out)
			}
		})
	}
	for _, flags := range [][]string{{"--timeout", "bad"}, {"--settle", "-1s"}, {"--timeout", "0s"}, {"--map=true"}} {
		out, code := e.Run(append([]string{"test", "run"}, flags...)...)
		if code != cmds.ExitUsage || !strings.HasPrefix(out, "error: ") {
			t.Fatalf("bad flags %v: %d, %s", flags, code, out)
		}
	}
}

func TestTestRunSettingsReport(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	e.AddWorkshop("1001", testkit.Sample("test.settingsowner", "SettingsOwner"))
	e.MustRun("scan")
	e.MustRun("enable", "pkg:test.settingsowner")
	var text strings.Builder
	for _, field := range []string{"windowPositionX", "windowPositionY", "scaleRelativeToSystem"} {
		fmt.Fprintf(&text, "[SettingsOwner] XML error: <%s>-27</%s> doesn't correspond to any field in type PrefsData\n", field, field)
	}
	text.WriteString("[SettingsOwner] XML error: <unknown>1</unknown> doesn't correspond to any field in type KeyPrefs\n")
	text.WriteString("[SettingsOwner] XML error: <bad>1</bad> doesn't correspond to any field in type ThingDef\n")
	e.App.TestGameStart = fakeTestGame(t, "menu", text.String())
	out, code := e.Run("test", "run", "--timeout", "5s", "--settle", "1ms", "--json")
	var r app.TestRunReport
	if code != 1 || json.Unmarshal([]byte(out), &r) != nil {
		t.Fatalf("exit %d: %s", code, out)
	}
	if r.Status != "menu" || r.Counts[gametest.ClassSettings] != 4 || r.Counts[logs.ClassXMLLoad] != 1 || r.Success() {
		t.Fatalf("settings report: %+v", r)
	}
	for _, c := range r.Clusters {
		if c.Class == gametest.ClassSettings && (len(c.Mods) != 0 || c.Confidence != "certain") {
			t.Fatalf("settings error attributed to a mod: %+v", c)
		}
		if c.Class == logs.ClassXMLLoad && !slices.Contains(c.Mods, "SettingsOwner") {
			t.Fatalf("ordinary XML attribution lost: %+v", c)
		}
	}
	e.App.TestGameStart = fakeTestGame(t, "menu", "XML error: unknown field in type PrefsData\n")
	out, code = e.Run("test", "run", "--timeout", "5s", "--settle", "1ms")
	// The only mods line is the gone ThingDef error's, attributed in the earlier report.
	if code != 1 || !strings.Contains(out, "settings x1:") || !strings.Contains(out, "source: game preferences") ||
		strings.Count(out, "mods:") != 1 || !strings.Contains(out, "mods: SettingsOwner (likely)") {
		t.Fatalf("settings text report: exit %d: %s", code, out)
	}
}

func TestTestRunCancelSavesReport(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := fakeTestGame(t, "idle", "")
	e.App.TestGameStart = func(cmd *exec.Cmd) error {
		err := start(cmd)
		cancel()
		return err
	}
	r, err := e.App.TestRun(ctx, app.TestRunOptions{})
	if !errors.Is(err, context.Canceled) || r == nil {
		t.Fatalf("report %+v, error %v", r, err)
	}
	if _, err := os.Stat(filepath.Join(r.RunDir, "report.json")); err != nil {
		t.Fatal(err)
	}
}

func linkNames(t *testing.T, mods string) []string {
	t.Helper()
	ents, err := os.ReadDir(mods)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range ents {
		if strings.HasPrefix(d.Name(), app.DeployPrefix) {
			out = append(out, d.Name())
		}
	}
	return out
}

// A Mod that would sit next to another copy of its packageId in Mods/ is not linked; a
// link an interrupted run left is removed by the next run (09 §7).
func TestTestRunLinksBlockedAndLeftover(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	extra := filepath.Join(e.Root, "extra")
	e.WriteMod(filepath.Join(extra, "Pinned"), testkit.Sample("test.pinned", "Pinned"))
	e.WriteMod(filepath.Join(extra, "Clash"), testkit.Sample("test.clash", "Clash"))
	e.WriteMod(filepath.Join(extra, "Old"), testkit.Sample("test.old", "Old"))
	e.MustRun("folder", "add", "extra", extra)
	e.MustRun("scan")
	e.MustRun("enable", "pkg:test.pinned")
	e.MustRun("enable", "pkg:test.clash")
	e.WriteMod(filepath.Join(e.Game, "Mods", "ClashCopy"), testkit.Sample("test.clash", "Clash copy"))
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(e.Home, "tests", st.Ix.Env.ID)
	mods := filepath.Join(e.Game, "Mods")
	leftover := filepath.Join(mods, app.DeployPrefix+"left")
	e.Junction(leftover, filepath.Join(extra, "Old"))
	keep := filepath.Join(mods, app.DeployPrefix+"retargeted")
	e.Junction(keep, filepath.Join(extra, "Old"))
	rec, _ := json.Marshal(map[string]any{"mods": mods, "links": []map[string]string{
		{"Op": "add", "Name": filepath.Base(leftover), "Target": filepath.Join(extra, "Old")},
		{"Op": "add", "Name": filepath.Base(keep), "Target": filepath.Join(extra, "Pinned")}, // no longer what was recorded
	}})
	e.WriteFile(filepath.Join(base, "links.json"), string(rec))
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{Timeout: 5 * time.Second, Settle: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Linked) != 1 || !strings.Contains(r.Linked[0], "Pinned") {
		t.Fatalf("linked %v", r.Linked)
	}
	if len(r.Missing) != 1 || r.Missing[0].PackageID != "test.clash" || !strings.Contains(r.Missing[0].Reason, "ClashCopy") {
		t.Fatalf("missing %+v", r.Missing)
	}
	if _, err := os.Lstat(leftover); !os.IsNotExist(err) {
		t.Fatalf("leftover link kept: %v", err)
	}
	if _, err := os.Lstat(keep); err != nil {
		t.Fatalf("removed a link that no longer matches the record: %v", err)
	}
	if left := linkNames(t, mods); !slices.Equal(left, []string{filepath.Base(keep)}) {
		t.Fatalf("links after the run: %v", left)
	}
}

// The flow the MCP Instructions recommend: pin_mod, edit the copy, then test_run tests the
// copy, not the Workshop original, and leaves Mods/ as it was (09 §7).
func TestTestRunSeesPinnedCopy(t *testing.T) {
	t.Parallel()
	e, _ := pinFixture(t)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	e.MustRun("pin", refOf(t, e, "Lockable", ""))
	before := e.ReadModsConfig()
	var during map[string]string
	start := fakeTestGame(t, "menu", "")
	e.App.TestGameStart = func(cmd *exec.Cmd) error {
		during = archoLinks(t, e)
		return start(cmd)
	}
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{Timeout: 5 * time.Second, Settle: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	lib := strings.ToLower(mustReal(t, library(e)))
	if len(r.Missing) != 0 || len(r.Linked) != 1 || len(during) != 1 || !strings.HasPrefix(strings.ToLower(during[r.Linked[0]]), lib) {
		t.Fatalf("missing %+v, linked %v, links during the run %v", r.Missing, r.Linked, during)
	}
	if left := archoLinks(t, e); len(left) != 0 {
		t.Fatalf("links left after the run: %v", left)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	config, err := loadout.ReadModsConfig(filepath.Join(e.Home, "tests", st.Ix.Env.ID, "userdata", "Config", "ModsConfig.xml"))
	if err != nil || !slices.Contains(config.Active, "tester.lock") {
		t.Fatalf("test config %+v, %v", config, err)
	}
	if e.ReadModsConfig() != before {
		t.Fatal("a test run wrote the player's ModsConfig.xml")
	}
}
