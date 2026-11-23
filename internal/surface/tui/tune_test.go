package tui

import (
	"math"
	"runtime"
	"runtime/debug"
	"testing"
)

// A list of 1,292 enabled Mods keeps about 42 MB live in the TUI; under the old 40 MiB soft
// cap every allocation started a collection and the idle TUI kept a core busy (VM
// 2026-11-23). With the process tuned, a live heap above 40 MiB and the allocations of a
// few redraws must cost a handful of collections, not one per allocation (17 §7.3).
func TestTuneProcessLiveHeapAboveFortyMiB(t *testing.T) {
	e, _ := fixture(t)
	defer debug.SetGCPercent(debug.SetGCPercent(100))
	defer debug.SetMemoryLimit(math.MaxInt64)
	TuneProcess(e.App)

	live := make([][]byte, 48) // 48 MiB that stays reachable, like a big list's index
	for i := range live {
		live[i] = make([]byte, 1<<20)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < 200; i++ { // 200 short-lived 256 KiB buffers: 50 MiB of garbage
		sink = make([]byte, 256<<10)
	}
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(live)
	if n := after.NumGC - before.NumGC; n > 10 {
		t.Fatalf("%d collections for 50 MiB of garbage over a 48 MiB live heap (limit %d MiB)", n, MemoryLimit>>20)
	}
}

var sink []byte
