package gametest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testOptions(t *testing.T, mode string, waylith bool) Options {
	t.Helper()
	// Run resolves paths; resolve here too so 8.3 short TEMP names (TOUMAN~1) compare equal.
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	o := Options{
		GameRoot: filepath.Join(base, "game"), DataDir: filepath.Join(base, "data"),
		RunDir: filepath.Join(base, "run with spaces"), Timeout: 5 * time.Second,
		Settle: time.Millisecond, MapQuiet: time.Millisecond,
	}
	writeTestFile(t, filepath.Join(o.GameRoot, "RimWorldWin64.exe"), "fake")
	writeTestFile(t, filepath.Join(o.GameRoot, "RimWorld.exe"), "fake")
	version := "1.6.4871 rev1261"
	if waylith {
		version = "1.7.4985 rev1269"
		writeTestFile(t, filepath.Join(o.GameRoot, "Waylith.dll"), "fake")
	}
	writeTestFile(t, filepath.Join(o.GameRoot, "Version.txt"), version)
	writeTestFile(t, filepath.Join(o.DataDir, "Config", "ModsConfig.xml"), "<ModsConfigData/>")
	writeTestFile(t, filepath.Join(o.DataDir, "ShaderCache", "reused"), "cache")
	testExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	o.Start = func(cmd *exec.Cmd) error {
		if cmd.Dir != o.GameRoot {
			t.Errorf("working directory %q", cmd.Dir)
		}
		if _, err := os.Stat(filepath.Join(o.DataDir, "Config", "Knowledge.xml")); !os.IsNotExist(err) {
			t.Errorf("old knowledge marker not removed: %v", err)
		}
		cmd.Env = append(os.Environ(), "RIMARCHO_GAME_HELPER="+mode,
			"RIMARCHO_HELPER_DATA="+o.DataDir, "RIMARCHO_HELPER_LOG="+filepath.Join(o.RunDir, "Player.log"),
			fmt.Sprintf("RIMARCHO_HELPER_WAYLITH=%t", waylith))
		cmd.Path = testExe
		cmd.Args = []string{testExe, "-test.run=^TestGameHelper$"}
		return cmd.Start()
	}
	return o
}

func assertResult(t *testing.T, o Options, r *Result, status string) {
	t.Helper()
	if r == nil || r.Status != status {
		t.Fatalf("result %+v, want %s", r, status)
	}
	if r.TotalSeconds <= 0 || r.ExitCode == nil {
		t.Fatalf("missing timing or exit code: %+v", r)
	}
	b, err := os.ReadFile(filepath.Join(o.RunDir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored Result
	if err := json.Unmarshal(b, &stored); err != nil || !reflect.DeepEqual(&stored, r) {
		t.Fatalf("stored %+v differs from %+v: %v", stored, r, err)
	}
	for _, path := range []string{r.LogPath, filepath.Join(o.RunDir, "stdout.log"),
		filepath.Join(o.DataDir, "Config", "ModsConfig.xml"), filepath.Join(o.DataDir, "ShaderCache", "reused")} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("artifact %s: %v", path, err)
		}
	}
}

func TestRunMenu(t *testing.T) {
	for _, waylith := range []bool{false, true} {
		t.Run(fmt.Sprint(waylith), func(t *testing.T) {
			t.Parallel()
			o := testOptions(t, "menu", waylith)
			writeTestFile(t, filepath.Join(o.DataDir, "Config", "Knowledge.xml"), "old marker")
			o.ExtraArgs = []string{"-some-option", "value with spaces"}
			r, err := Run(context.Background(), o)
			if err != nil {
				t.Fatal(err)
			}
			assertResult(t, o, r, "menu")
			if r.Waylith != waylith || r.LoadSeconds <= 0 || r.TotalSeconds <= r.LoadSeconds {
				t.Fatalf("result: %+v", r)
			}
			want := []string{"-savedatafolder=" + o.DataDir}
			if waylith {
				want = append(want, "-NoFileLog")
				if filepath.Base(r.Exe) != "RimWorld.exe" {
					t.Fatalf("exe %q", r.Exe)
				}
			} else {
				want = append(want, "-logfile", r.LogPath, "-popupwindow")
				if filepath.Base(r.Exe) != "RimWorldWin64.exe" {
					t.Fatalf("exe %q", r.Exe)
				}
			}
			want = append(want, o.ExtraArgs...)
			if !reflect.DeepEqual(want, r.Args) {
				t.Fatalf("args %q, want %q", r.Args, want)
			}
			b, err := os.ReadFile(r.LogPath)
			if err != nil || !strings.Contains(string(b), "Late error") {
				t.Fatalf("late log %q: %v", b, err)
			}
			if waylith {
				f, err := logs.Parse(strings.NewReader(string(b)))
				if err != nil || !f.Waylith || f.Version != "1.7.4985 rev1269" || len(f.Entries) != 2 || f.Entries[1].Level != "ERROR" {
					t.Fatalf("log %+v: %v", f, err)
				}
			}
			capture, _ := os.ReadFile(filepath.Join(o.RunDir, "stdout.log"))
			if !strings.Contains(string(capture), "stdout captured") || !strings.Contains(string(capture), "stderr captured") {
				t.Fatalf("capture %q", capture)
			}
		})
	}
}

func TestRunTimeout(t *testing.T) {
	t.Parallel()
	o := testOptions(t, "idle", false)
	o.Timeout = 650 * time.Millisecond
	// A newly written file with an old mtime is not a menu marker.
	o.Start = staleMarkerStarter(o.Start, o.DataDir)
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, o, r, "timeout")
	if r.LoadSeconds != 0 {
		t.Fatalf("stale marker accepted: %+v", r)
	}
}

func staleMarkerStarter(start func(*exec.Cmd) error, dataDir string) func(*exec.Cmd) error {
	return func(cmd *exec.Cmd) error {
		if err := start(cmd); err != nil {
			return err
		}
		path := filepath.Join(dataDir, "Config", "Knowledge.xml")
		if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
			panic(err)
		}
		old := time.Now().Add(-time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			panic(err)
		}
		return nil
	}
}

func TestRunExited(t *testing.T) {
	t.Parallel()
	o := testOptions(t, "exit", true)
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, o, r, "exited")
	if *r.ExitCode != 7 {
		t.Fatalf("exit code %d", *r.ExitCode)
	}
}

func TestRunMap(t *testing.T) {
	for _, waylith := range []bool{false, true} {
		t.Run(fmt.Sprint(waylith), func(t *testing.T) {
			t.Parallel()
			o := testOptions(t, "map", waylith)
			o.Map = true
			o.MapQuiet = 600 * time.Millisecond
			r, err := Run(context.Background(), o)
			if err != nil {
				t.Fatal(err)
			}
			assertResult(t, o, r, "map")
			if !strings.Contains(strings.Join(r.Args, " "), "quicktest") || r.LoadSeconds < 2 {
				t.Fatalf("map did not wait for late activity: %+v", r)
			}
		})
	}
}

// A save run copies the save in as autostart.rws, turns dev mode on and stops at the first
// autosave; a map run with ticks does the same on a test map without dev mode (20 §2).
func TestRunSaveAndTicks(t *testing.T) {
	for _, mode := range []string{"save", "ticks"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			o := testOptions(t, mode, false)
			o.Ticks = 3000
			o.MapQuiet = time.Hour // the autosave decides, not a quiet log
			writeTestFile(t, filepath.Join(o.DataDir, "Saves", "Autosave-3.rws"), "an earlier run's autosave")
			writeTestFile(t, filepath.Join(o.DataDir, "Saves", "keep.rws"), "other save")
			if mode == "save" {
				o.Save = filepath.Join(t.TempDir(), "Colony.rws")
				writeTestFile(t, o.Save, "<savegame>colony</savegame>")
			} else {
				o.Map = true
				writeTestFile(t, filepath.Join(o.DataDir, "Saves", "autostart.rws"), "stale")
			}
			r, err := Run(context.Background(), o)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"save": "save", "ticks": "map"}[mode]
			assertResult(t, o, r, want)
			phase := map[string]string{"save": "save", "ticks": "new"}[mode]
			if r.TicksReached != 3000 || r.Phase != phase || r.MapSeconds < 0.5 || r.LoadSeconds < r.MapSeconds || r.LastLine != "" {
				t.Fatalf("result %+v", r)
			}
			if slices.Contains(r.Args, "quicktest") != (mode == "ticks") {
				t.Fatalf("args %q", r.Args)
			}
			prefs, _ := os.ReadFile(filepath.Join(o.DataDir, "Config", "Prefs.xml"))
			if !strings.Contains(string(prefs), "<autosaveIntervalDays>0.05</autosaveIntervalDays>") ||
				strings.Contains(string(prefs), "<devMode>True</devMode>") != (mode == "save") ||
				!strings.Contains(string(prefs), "<runInBackground>True</runInBackground>") {
				t.Fatalf("prefs %s", prefs)
			}
			autostart, err := os.ReadFile(filepath.Join(o.DataDir, "Saves", "autostart.rws"))
			if mode == "save" && string(autostart) != "<savegame>colony</savegame>" || mode == "ticks" && !os.IsNotExist(err) {
				t.Fatalf("autostart %q, %v", autostart, err)
			}
			if _, err := os.Stat(filepath.Join(o.DataDir, "Saves", "Autosave-3.rws")); !os.IsNotExist(err) {
				t.Fatalf("an earlier autosave stayed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(o.DataDir, "Saves", "keep.rws")); err != nil {
				t.Fatal(err)
			}
		})
	}
	o := testOptions(t, "idle", false)
	o.Save, o.Map = "x.rws", true
	if _, err := Run(context.Background(), o); err == nil {
		t.Fatal("a save run with a test map accepted")
	}
}

// A run that does not arrive reports the furthest phase and the log's last line.
func TestRunTimeoutLastLine(t *testing.T) {
	t.Parallel()
	o := testOptions(t, "map", false)
	o.Map = true
	o.MapQuiet = time.Hour
	o.Timeout = 1200 * time.Millisecond
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, o, r, "timeout")
	if r.Phase != "new" || r.LastLine != "03:36:13  INFO Map still loading" || r.MapSeconds <= 0 {
		t.Fatalf("result %+v", r)
	}
}

func TestRunProbe(t *testing.T) {
	for _, tc := range []struct {
		mode    string
		waylith bool
		status  string
		quit    bool // the game ends by itself (0, or 7 for the helper's early exit)
	}{
		{"probe-quit", false, "probe", true},
		{"probe-quit", true, "probe", true},
		{"probe-stay", false, "probe", false},
		{"probe-exception", true, "probe", false},
		{"exit", false, "exited", true},
	} {
		t.Run(fmt.Sprint(tc.mode, tc.waylith), func(t *testing.T) {
			t.Parallel()
			o := testOptions(t, tc.mode, tc.waylith)
			o.Probe = true
			o.Settle = 400 * time.Millisecond
			r, err := Run(context.Background(), o)
			if err != nil {
				t.Fatal(err)
			}
			assertResult(t, o, r, tc.status)
			if !strings.Contains(strings.Join(r.Args, " "), "quicktest "+ProbeQuit) {
				t.Fatalf("args %q", r.Args)
			}
			if tc.quit != (*r.ExitCode == 0 || *r.ExitCode == 7) {
				t.Fatalf("exit code %d, quit %v", *r.ExitCode, tc.quit)
			}
			if tc.status == "probe" && (r.LoadSeconds < 0.5 || r.TotalSeconds < r.LoadSeconds) {
				t.Fatalf("timing %+v", r)
			}
			if !tc.quit && r.TotalSeconds-r.LoadSeconds < 0.3 {
				t.Fatalf("did not wait for the probe to quit: %+v", r)
			}
		})
	}
	o := testOptions(t, "idle", false)
	o.ExtraArgs = []string{"-probequit"}
	if _, err := Run(context.Background(), o); err == nil || !strings.Contains(err.Error(), "managed option") {
		t.Fatalf("managed -probequit accepted: %v", err)
	}
}

func TestProbeLine(t *testing.T) {
	for _, tc := range []struct{ line, text, end string }{
		{"[Probe] RESULT smoke ok errors=[setup:0]\r", "RESULT smoke ok errors=[setup:0]", "result"},
		{"[Probe] RESULT", "RESULT", "result"},
		{"2026-11-05 03:36:12.000  INFO [Probe] EXCEPTION in Setup System.Exception: x", "EXCEPTION in Setup System.Exception: x", "exception"},
		{"2026-11-05 03:36:12.000  INFO [Probe] RESULTS later", "RESULTS later", ""},
		{"[Probe] t=250 a:Wait", "t=250 a:Wait", ""},
	} {
		text, ok := ProbeLine(tc.line)
		if !ok || text != tc.text || ProbeEnd(text) != tc.end {
			t.Errorf("%q: %q %v %q", tc.line, text, ok, ProbeEnd(text))
		}
	}
	for _, line := range []string{"Error: [Probe] RESULT inside", " [Probe] RESULT", "2026-11-05 03:36:12.000  INFO Mod [Probe] RESULT x"} {
		if _, ok := ProbeLine(line); ok {
			t.Errorf("%q read as a probe line", line)
		}
	}
}

func TestRunCancel(t *testing.T) {
	t.Parallel()
	o := testOptions(t, "idle", true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := o.Start
	o.Start = func(cmd *exec.Cmd) error {
		err := start(cmd)
		cancel()
		return err
	}
	r, err := Run(ctx, o)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error %v", err)
	}
	assertResult(t, o, r, "timeout")
}

func TestRunStartFailed(t *testing.T) {
	o := testOptions(t, "idle", false)
	want := errors.New("fake start failure")
	o.Start = func(*exec.Cmd) error { return want }
	r, err := Run(context.Background(), o)
	if !errors.Is(err, want) || r == nil || r.Status != "start-failed" || r.ExitCode != nil {
		t.Fatalf("result %+v, error %v", r, err)
	}
	if _, err := os.Stat(filepath.Join(o.RunDir, "result.json")); err != nil {
		t.Fatal(err)
	}
}

func TestGameHelper(t *testing.T) {
	mode := os.Getenv("RIMARCHO_GAME_HELPER")
	if mode == "" {
		return
	}
	var log io.Writer = os.Stdout
	if os.Getenv("RIMARCHO_HELPER_WAYLITH") != "true" {
		f, err := os.OpenFile(os.Getenv("RIMARCHO_HELPER_LOG"), os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			os.Exit(99)
		}
		defer f.Close()
		log = f
	}
	fmt.Fprintln(os.Stdout, "stdout captured")
	fmt.Fprintln(os.Stderr, "stderr captured")
	fmt.Fprint(log, "03:36:12  INFO Loading\r\n")
	switch mode {
	case "exit":
		os.Exit(7)
	case "menu":
		time.Sleep(30 * time.Millisecond)
		path := filepath.Join(os.Getenv("RIMARCHO_HELPER_DATA"), "Config", "Knowledge.xml")
		if err := os.WriteFile(path, []byte("menu"), 0o644); err != nil {
			os.Exit(98)
		}
		time.Sleep(600 * time.Millisecond)
		fmt.Fprint(log, "03:36:13  ERROR Late error\r\n")
	case "map":
		fmt.Fprintf(log, "03:36:12  INFO %s\r\n", mapMarker)
		time.Sleep(700 * time.Millisecond)
		fmt.Fprint(log, "03:36:13  INFO Map still loading\r\n")
	case "save", "ticks":
		// The game's load or new-game marker, then the first autosave after the ticks.
		if mode == "save" {
			fmt.Fprint(log, "03:36:12  INFO Loading game from file autostart with mods:\r\n  - ludeon.rimworld\r\n")
		} else {
			fmt.Fprintf(log, "03:36:12  INFO %s:\r\n  - ludeon.rimworld\r\n", mapMarker)
		}
		time.Sleep(700 * time.Millisecond)
		path := filepath.Join(os.Getenv("RIMARCHO_HELPER_DATA"), "Saves", "Autosave-1.rws")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			os.Exit(95)
		}
		if err := os.WriteFile(path, []byte("<savegame/>"), 0o644); err != nil {
			os.Exit(94)
		}
	case "probe-quit", "probe-stay", "probe-exception":
		// Unity writes the message alone; the 1.7 console writes time and level first.
		line := func(s string) {
			if os.Getenv("RIMARCHO_HELPER_WAYLITH") == "true" {
				fmt.Fprintf(log, "03:36:14  INFO %s\r\n", s)
			} else {
				fmt.Fprintf(log, "%s\r\n", s)
			}
		}
		fmt.Fprintf(log, "03:36:12  INFO %s\r\n", mapMarker)
		line("[Probe] startup scenario=Scenario")
		time.Sleep(700 * time.Millisecond)
		line("[Probe] RESULTS are not the end")
		if mode == "probe-exception" {
			line("[Probe] EXCEPTION in Setup System.Exception: boom")
		} else {
			line("[Probe] RESULT smoke ok")
		}
		if mode == "probe-quit" {
			os.Exit(0)
		}
	}
	time.Sleep(30 * time.Second)
}
