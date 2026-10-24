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
	}
	time.Sleep(30 * time.Second)
}
