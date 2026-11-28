package app_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	testhide "github.com/ToumanLin/RimArcho/internal/testkit/hide"
)

// TestFirstAnalysisLockHelper is the other process of TestFirstAnalysisDuringTestRun: it holds
// the launch lock with a test run's busy.json until the stop file appears.
func TestFirstAnalysisLockHelper(t *testing.T) {
	home := os.Getenv("RIMARCHO_TEST_LOCK_HOME")
	if home == "" {
		return
	}
	lock, err := app.LockWorkspace(home, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	rec, _ := json.Marshal(map[string]any{"task": "a test run of plan:Colony (Game Config main)", "kind": "test-run",
		"pid": os.Getpid(), "started": time.Now(), "until": time.Now().Add(10 * time.Minute)})
	busy := filepath.Join(home, "busy.json")
	if err := os.WriteFile(busy, rec, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(busy)
	if err := os.WriteFile(filepath.Join(home, "lock-ready"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(home, "lock-stop")); err == nil {
			return
		}
	}
	t.Fatal("no stop file")
}

// TestFirstAnalysisDuringTestRun: while another process's test run holds the launch lock, a
// stored list's first analysis reads the stored list and runs; one that would read a Mods/
// entry the run moved aside waits and says so (14 §1 "Busy").
func TestFirstAnalysisDuringTestRun(t *testing.T) {
	e := testkit.New(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lamp"), testkit.Sample("test.lamp", "Lamp"))
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout() // the list is stored, never analysed
	if err != nil {
		t.Fatal(err)
	}
	if s, err := e.App.Analysis(st); err != nil || s.Run != nil {
		t.Fatalf("analysis before the run: %+v %v", s, err)
	}

	cmd := testhide.Cmd(exec.Command(os.Args[0], "-test.run=^TestFirstAnalysisLockHelper$", "-test.count=1"))
	cmd.Env = append(os.Environ(), "RIMARCHO_TEST_LOCK_HOME="+e.Home)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			os.WriteFile(filepath.Join(e.Home, "lock-stop"), nil, 0o644)
			cmd.Wait()
		}
	}
	defer stop()
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(e.Home, "lock-ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the helper did not take the lock")
		}
	}
	child, err := e.ChildApp()
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()

	// The run moved Mods/Lamp aside: the analysis reads it, so it waits.
	journal := filepath.Join(e.Home, "tests", "env1", "links.json")
	e.WriteFile(journal, `{"mods": `+jsonString(filepath.Join(e.Game, "Mods"))+`, "links": [], "hidden": [{"name": "Lamp", "to": "x", "mod": "y"}]}`)
	if _, err := child.Analyze(nil); err == nil || !strings.Contains(err.Error(), "busy: a test run of plan:Colony") || !strings.Contains(err.Error(), "Mods/Lamp, which the test run moved aside") {
		t.Fatalf("analysis with a Mod moved aside: %v", err)
	}
	e.Remove(journal)

	if _, err := child.Analyze(nil); err != nil {
		t.Fatalf("first analysis during another process's test run: %v", err)
	}
	stop()
	st, err = e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if s, err := e.App.Analysis(st); err != nil || s.Run == nil || !s.Fresh {
		t.Fatalf("analysis after the run: %+v %v", s, err)
	}
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }
