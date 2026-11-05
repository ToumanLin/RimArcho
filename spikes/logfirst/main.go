// Spike: the first query on a new Player.log (18, target ≤ 500 ms). Copies a log to a
// temporary file several times (each copy is a log the cache has never seen) and times
// App.AnalyzeLog on each, after one warm-up that brings the analysis for the log's own game
// version up to date. Use a data directory COPY: the reports are stored in its log_cache.
//
//	go run ./spikes/logfirst -home <copy of %APPDATA%\RimArcho> -log <Player.log> [-cpuprofile f]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func main() {
	home := flag.String("home", "", "RimArcho data directory (a copy)")
	logPath := flag.String("log", "", "a Player.log")
	runs := flag.Int("runs", 5, "timed runs")
	cpu := flag.String("cpuprofile", "", "write a CPU profile of the timed runs")
	dump := flag.String("dump", "", "write the warm-up report's clusters (class, count, candidates, confidence) as JSON")
	flag.Parse()
	a, err := app.Open(*home)
	must(err)
	a.Launcher = func(string, []string, string) error { panic("launch attempted") }
	st, err := a.Loadout()
	must(err)
	dir, err := os.MkdirTemp("", "logfirst")
	must(err)
	defer os.RemoveAll(dir)
	copyTo := func(i int) string {
		p := filepath.Join(dir, fmt.Sprintf("run%d", i), "Player.log")
		must(os.MkdirAll(filepath.Dir(p), 0o755))
		src, err := os.Open(*logPath)
		must(err)
		defer src.Close()
		dst, err := os.Create(p)
		must(err)
		_, err = io.Copy(dst, src)
		must(err)
		must(dst.Close())
		return p
	}
	t0 := time.Now()
	must(a.EnsureLogAnalysis(st, app.LogVersionOf(*logPath), nil))
	r, err := a.AnalyzeLog(copyTo(0))
	must(err)
	if *dump != "" {
		b, _ := json.MarshalIndent(r.Clusters, "", " ")
		must(os.WriteFile(*dump, b, 0o644))
	}
	fmt.Printf("warm-up (analysis + first report): %v, %d lines, %d clusters\n", time.Since(t0).Round(time.Millisecond), r.Lines, len(r.Clusters))
	if *cpu != "" {
		f, err := os.Create(*cpu)
		must(err)
		must(pprof.StartCPUProfile(f))
		defer pprof.StopCPUProfile()
	}
	var ds []time.Duration
	for i := 1; i <= *runs; i++ {
		p := copyTo(i)
		a.DropLogCaches()
		t := time.Now()
		_, err := a.AnalyzeLog(p)
		must(err)
		ds = append(ds, time.Since(t))
	}
	fmt.Printf("first query on a new log: %v\n", ds)
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	fmt.Printf("median %d ms, max %d ms\n", ds[len(ds)/2].Milliseconds(), ds[len(ds)-1].Milliseconds())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
