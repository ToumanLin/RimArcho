// Spike: run the M3 Def+Patch analysis against a real data directory without persisting
// anything, report time and memory, and replay Player.log patch failures against the
// simulation (03 §7 acceptance, as spikes/xpathcompat did in M0). Read-only.
//
//	go run ./spikes/analyzereal -home %APPDATA%\RimArcho -version 1.6
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/rimarcho/rimarcho/internal/analyze"
	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/env"
	"github.com/rimarcho/rimarcho/spikes/internal/procmem"
)

func main() {
	home := flag.String("home", "", "RimArcho data directory")
	version := flag.String("version", "", "major.minor to analyse for (default: the environment's)")
	logPath := flag.String("log", "", "Player.log (default: the environment's)")
	memLimit := flag.Int("memlimit", 0, "soft memory limit in MB (0: none)")
	gcPct := flag.Int("gc", 0, "GOGC (0: default)")
	memProf := flag.String("memprofile", "", "write a heap profile after the run")
	flag.Parse()
	a, err := app.Open(*home)
	must(err)
	a.Launcher = func(string, []string, string) error { panic("launch attempted") }
	st, err := a.Loadout()
	must(err)
	in, key, err := a.AnalyzeInput(st, *version)
	must(err)
	in.Progress = func(s string) { fmt.Println("  " + s) }
	gamePath := map[string]string{}
	for _, m := range in.Mods {
		gamePath[m.Instance] = m.Dir
	}
	if *memLimit > 0 {
		debug.SetMemoryLimit(int64(*memLimit) << 20)
	}
	if *gcPct > 0 {
		debug.SetGCPercent(*gcPct)
	}
	if *memProf != "" {
		in.AfterSimulation = func() {
			f, _ := os.Create(*memProf)
			pprof.WriteHeapProfile(f)
			f.Close()
		}
	}
	t0 := time.Now()
	out, err := analyze.Run(in)
	must(err)
	el := time.Since(t0)
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	ws, priv, peak := procmem.MB()
	s := out.Stats
	fmt.Printf("run key %s, version %s\n", key, in.MM)
	fmt.Printf("mods: %d active, %d installed; def files %d, defs %d; patch files %d, top-level ops %d (unknown classes %d)\n",
		s.ActiveMods, s.InstalledMods, s.DefFiles, s.Defs, s.PatchFiles, s.TopOps, s.UnknownOps)
	fmt.Printf("xml parse errors %d; nodes %d, text %d MB; index hits %d rebuilds %d; evaluations %d\n",
		s.ParseErrors, s.Nodes, s.TextMB, s.IndexHits, s.IndexRebuilds, s.Evaluations)
	fmt.Printf("time: defs %d ms, patches+findings %d ms, total %v\n", s.DefsMs, s.PatchMs, el.Round(time.Millisecond))
	fmt.Printf("live heap after GC %d MB; ", s.LiveHeapMB)
	fmt.Printf("memory: heap in use at end of simulation %d MB, sys %d MB; working set %.0f MB, private %.0f MB, PEAK working set %.0f MB\n",
		s.PeakHeapMB, ms.Sys>>20, ws, priv, peak)
	byCode := map[string]int{}
	for _, f := range out.Findings {
		byCode[f.Code]++
	}
	var codes []string
	for c, n := range byCode {
		codes = append(codes, fmt.Sprintf("%s %d", c, n))
	}
	sort.Strings(codes)
	fmt.Printf("findings: %s\n", strings.Join(codes, ", "))
	reasons := map[string]int{}
	for _, p := range out.Patches {
		for _, r := range p.Reasons {
			reasons[r]++
		}
	}
	fmt.Printf("no-match reasons: %v\n", reasons)

	// Player.log replay.
	if *logPath == "" {
		*logPath = env.PlayerLog(st.Ix.Env.UserDataRoot, in.MM)
	}
	lf, err := analyze.ParseLogFailures(*logPath)
	if err != nil {
		fmt.Println("no Player.log:", err)
		return
	}
	li, _ := os.Stat(*logPath)
	logCnt := map[string]int{}
	for _, l := range lf {
		logCnt[l.Key()]++
	}
	var onlySim, onlyLog []string
	agree := 0
	simFailed := 0
	for _, p := range out.Patches {
		if p.Result != analyze.ResultFailed {
			continue
		}
		simFailed++
		path := filepath.Join(gamePath[p.Instance], filepath.FromSlash(p.File))
		k := analyze.FailKey(path, p.Desc)
		if logCnt[k] > 0 {
			logCnt[k]--
			agree++
			continue
		}
		tag := ""
		if fi, err := os.Stat(path); err == nil && fi.ModTime().After(li.ModTime()) {
			tag = " [file changed after the log]"
		}
		onlySim = append(onlySim, fmt.Sprintf("%s %s (%s)%s", path, p.Desc, strings.Join(p.Reasons, ","), tag))
	}
	for _, l := range lf {
		if logCnt[l.Key()] > 0 {
			logCnt[l.Key()]--
			onlyLog = append(onlyLog, fmt.Sprintf("[%s] %s  %s", l.Mod, l.Desc, l.File))
		}
	}
	fmt.Printf("\nPlayer.log %s (%s): %d failures; simulated %d; agreeing %d; sim-only %d; log-only %d; mismatch rate %.3f%% of %d ops\n",
		*logPath, li.ModTime().Format(time.RFC3339), len(lf), simFailed, agree, len(onlySim), len(onlyLog),
		100*float64(len(onlySim)+len(onlyLog))/float64(max(1, s.TopOps)), s.TopOps)
	fmt.Printf("== simulated, not in the log ==\n  %s\n", strings.Join(onlySim, "\n  "))
	fmt.Printf("== in the log, not simulated ==\n  %s\n", strings.Join(onlyLog, "\n  "))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
