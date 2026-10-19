package tui

// Measurement against a real installation (01 §5.2, 06 §8 T5), skipped unless
// RIMARCHO_REAL_HOME (data dir) and RIMARCHO_EXE (archo binary for the child scan) are set.
// Only navigation and search keys are sent; the launcher fails the test if called.

import (
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/procmem"
)

func TestRealIdleMemory(t *testing.T) {
	home, exe := os.Getenv("RIMARCHO_REAL_HOME"), os.Getenv("RIMARCHO_EXE")
	if home == "" || exe == "" {
		t.Skip("set RIMARCHO_REAL_HOME and RIMARCHO_EXE to measure a real installation")
	}
	idle := 10 * time.Minute
	if s := os.Getenv("RIMARCHO_IDLE_SECONDS"); s != "" {
		n, _ := strconv.Atoi(s)
		idle = time.Duration(n) * time.Second
	}
	a, err := app.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Launcher = func(string, []string, string) error { t.Fatal("launch attempted during measurement"); return nil }

	debug.SetMemoryLimit(MemoryLimit) // as Run does
	ws0, _, _ := procmem.MB()
	t.Logf("before TUI: working set %.1f MB", ws0)
	if _, err := a.Loadout(); err != nil {
		t.Fatal(err)
	}
	ws1, _, _ := procmem.MB()
	t.Logf("after one workspace load: working set %.1f MB", ws1)
	opts := Options{App: a, Scan: func(env string) (string, error) {
		out, err := exec.Command(exe, "scan").CombinedOutput()
		return string(out), err
	}}
	tm := teatest.NewTestModel(t, New(opts), teatest.WithInitialTermSize(120, 30))
	waitFor(t, tm, "active 485/")
	if os.Getenv("RIMARCHO_NO_BROWSE") == "" {
		keys(tm, "R")
		waitFor(t, tm, "scan:")
		for i := 0; i < 60; i++ {
			keys(tm, "j")
			time.Sleep(150 * time.Millisecond) // let the detail pane load for every row
		}
		keys(tm, "/", "har", "enter", "esc", "G", "g", "f", "f", "f", "f")
	}
	time.Sleep(2 * time.Second)
	ws, priv, peak := procmem.MB()
	t.Logf("TUI after load, child scan and browsing: working set %.1f MB, private %.1f MB, peak %.1f MB", ws, priv, peak)
	step := idle / 5
	for i := 1; i <= 5; i++ {
		time.Sleep(step)
		ws, priv, peak = procmem.MB()
		t.Logf("idle %v: working set %.1f MB, private %.1f MB, peak %.1f MB", time.Duration(i)*step, ws, priv, peak)
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		t.Logf("  go heap in use %.1f MB, heap sys %.1f MB, total go sys %.1f MB", float64(ms.HeapInuse)/(1<<20), float64(ms.HeapSys)/(1<<20), float64(ms.Sys)/(1<<20))
	}
	finish(t, tm)

	// Sort timing on the real workspace (dry run: nothing changes).
	var best time.Duration
	for i := 0; i < 10; i++ {
		st, err := a.Loadout()
		if err != nil {
			t.Fatal(err)
		}
		in, err := a.SortInput(st, st.List)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		r, err := a.Sort(true)
		if err != nil {
			t.Fatal(err)
		}
		el := time.Since(start)
		if best == 0 || el < best {
			best = el
		}
		if i == 0 {
			t.Logf("sort: %d entries, %d edges, %d cycles", len(in.Nodes), len(r.Result.Edges), len(r.Result.Cycles))
		}
	}
	t.Logf("sort (load workspace + build graph + sort + diff), best of 10: %v", best)

}
