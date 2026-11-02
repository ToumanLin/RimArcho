package view_test

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Synthetic fixture only: one searchable XML file in each of 1,354 installed Mods.
func TestFileSearch1354Measurement(t *testing.T) {
	if os.Getenv("ARCHO_FILESEARCH_BENCH") != "1" {
		t.Skip("set ARCHO_FILESEARCH_BENCH=1")
	}
	e := testkit.New(t)
	pkgs := []string{"ludeon.rimworld"}
	for i := 0; i < 1353; i++ {
		pkg := fmt.Sprintf("bench.mod%d", i)
		e.AddWorkshop(fmt.Sprint(10000+i), testkit.Sample(pkg, fmt.Sprintf("Mod %04d", i)))
		pkgs = append(pkgs, pkg)
	}
	e.WriteModsConfig("1.6.4633 rev1261", pkgs...)
	e.AddEnv(false)
	e.MustRun("scan")
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	lib, err := e.App.FileSearchLibrary()
	if err != nil {
		t.Fatal(err)
	}
	info, err := view.BuildFileSearchInfo(lib, "", true)
	opened := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	run := view.NewFileSearchRun()
	done := make(chan struct{})
	go func() {
		defer close(done)
		run.Search(context.Background(), e.App, view.FileSearchReq{Pattern: "Thing", ModsQuery: info.Mods})
	}()
	var first time.Duration
	var peak uint64
	since := 0
	for {
		u := run.Update(since)
		since = u.Next
		if first == 0 && u.Matches > 0 {
			first = time.Since(start)
		}
		var sample runtime.MemStats
		runtime.ReadMemStats(&sample)
		peak = max(peak, sample.HeapAlloc)
		if u.Done {
			if u.Error != "" {
				t.Fatal(u.Error)
			}
			break
		}
		time.Sleep(time.Millisecond)
	}
	<-done
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	t.Logf("stream page=%s first=%s full=%s heap=%.2fMiB peakHeap=%.2fMiB allocated=%.2fMiB mods=%d", opened, first, elapsed, float64(after.HeapAlloc)/1048576, float64(peak)/1048576, float64(after.TotalAlloc-before.TotalAlloc)/1048576, run.Update(0).Mods)
}
