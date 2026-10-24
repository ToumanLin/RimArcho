package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/fsscan"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

type totals struct {
	dirs, files, bytes, checksum    uint64
	errors, skipped, hashed, reused int
	firstError                      string
}

func (t *totals) add(o totals) {
	t.dirs += o.dirs
	t.files += o.files
	t.bytes += o.bytes
	t.checksum += o.checksum
	t.errors += o.errors
	t.skipped += o.skipped
	t.hashed += o.hashed
	t.reused += o.reused
	if t.firstError == "" {
		t.firstError = o.firstError
	}
}
func (t *totals) fail(err error) {
	if err != nil {
		t.errors++
		if t.firstError == "" {
			t.firstError = err.Error()
		}
	}
}
func (t *totals) file(size, mtime uint64) { t.files++; t.bytes += size; t.checksum += size + mtime }

var skippedNames = map[string]bool{".git": true, ".svn": true, ".rimarcho": true}

func skipDir(n string) bool { return skippedNames[n] }

type snapshot struct {
	dirs   []string
	links  bool
	totals totals
}

func walk(root string, capture *snapshot) totals {
	var t totals
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			t.fail(err)
			return nil
		}
		if d.IsDir() && p != root && skipDir(d.Name()) {
			return filepath.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			t.fail(err)
			return nil
		}
		if isReparse(info) {
			if capture != nil {
				capture.links = true
			}
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			t.dirs++
			if capture != nil {
				capture.dirs = append(capture.dirs, p)
			}
		} else if info.Mode().IsRegular() {
			t.file(uint64(info.Size()), uint64(info.ModTime().UnixNano()))
		}
		return nil
	})
	t.fail(err)
	if capture != nil {
		capture.totals = t
	}
	return t
}

// Results are reduced per worker, keeping the hot path free of shared counters.
func parallel(n, workers int, job func(int) totals) totals {
	jobs := make(chan int)
	results := make(chan totals, workers)
	for w := 0; w < workers; w++ {
		go func() {
			var t totals
			for i := range jobs {
				t.add(job(i))
			}
			results <- t
		}()
	}
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)
	var t totals
	for w := 0; w < workers; w++ {
		t.add(<-results)
	}
	return t
}

// pending includes queued and in-flight directories. Workers never block sending
// children to a bounded queue while all other workers are also producing children.
func dirQueue(mods []string, workers int) totals {
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	queue := append([]string(nil), mods...)
	pending := len(queue)
	results := make(chan totals, workers)
	for w := 0; w < workers; w++ {
		go func() {
			var t totals
			for {
				mu.Lock()
				for len(queue) == 0 && pending != 0 {
					cond.Wait()
				}
				if pending == 0 {
					mu.Unlock()
					break
				}
				dir := queue[len(queue)-1]
				queue = queue[:len(queue)-1]
				mu.Unlock()
				children := []string{}
				f, err := openDirectory(dir)
				if err == nil {
					t.dirs++
					entries, readErr := f.ReadDir(-1)
					t.fail(readErr)
					t.fail(f.Close())
					for _, e := range entries {
						if e.IsDir() && skipDir(e.Name()) {
							continue
						}
						info, err := e.Info()
						if err != nil {
							t.fail(err)
							continue
						}
						if isReparse(info) {
							continue
						}
						if e.IsDir() {
							children = append(children, filepath.Join(dir, e.Name()))
						} else if info.Mode().IsRegular() {
							t.file(uint64(info.Size()), uint64(info.ModTime().UnixNano()))
						}
					}
				} else {
					t.fail(err)
				}
				mu.Lock()
				queue = append(queue, children...)
				pending += len(children) - 1
				cond.Broadcast()
				mu.Unlock()
			}
			results <- t
		}()
	}
	var t totals
	for w := 0; w < workers; w++ {
		t.add(<-results)
	}
	return t
}

var modeNames = []string{"walkdir", "walkdir-par", "dirqueue-par", "fsscan-walk", "fsscan-size", "findex", "fileinfo-big", "scan-compute", "scan-compute-prev", "tex-fingerprint", "about", "dirstat", "usn", "open-cold"}

func main() {
	roots := flag.String("roots", `D:\SteamLibrary\steamapps\workshop\content\294100;D:\SteamLibrary\steamapps\common\RimWorld\Mods`, "Parent folders separated by semicolons; immediate subfolders are mods")
	limit := flag.Int("limit", 0, "First N mods in root/name order (0 = all)")
	runs := flag.Int("runs", 3, "Runs per mode and worker count")
	modesFlag := flag.String("modes", "all", "Comma-separated modes: "+strings.Join(modeNames, ","))
	workersFlag := flag.String("workers", "1,2,4,8,16,32", "Comma-separated positive worker counts for parallel modes")
	seed := flag.Int64("seed", 1, "Deterministic mod shuffle seed for open-cold")
	groupMods := flag.Int("groupmods", 150, "Mods per worker-count group for open-cold")
	skipMods := flag.Int("skipmods", 0, "Skip N mods of the shuffled open-cold order; keep roots, limit, and seed unchanged across batches")
	rewarm := flag.Bool("rewarm", false, "Repeat open-cold groups once in a second, warm table")
	ext := flag.String("ext", ".png", "Case-insensitive file extension for open-cold (for example .png or .xml)")
	prepareBaseline := flag.Bool("baseline", true, "Prepare an untimed reference walk for totals checks; disable for standalone profiles")
	skipDirs := flag.String("skipdirs", ".git,.svn,.rimarcho", "Comma-separated directory base names to skip; empty skips none (fsscan-size is always unfiltered)")
	cpuProfile := flag.String("cpuprofile", "", "Write a CPU profile of the selected modes to this file")
	memProfile := flag.String("memprofile", "", "Write a heap and allocation profile to this file")
	flag.Parse()
	skippedNames = map[string]bool{}
	for _, name := range strings.Split(*skipDirs, ",") {
		if name = strings.TrimSpace(name); name != "" {
			skippedNames[name] = true
		}
	}
	fatal := func(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	if *limit < 0 || *runs < 1 || flag.NArg() != 0 {
		fatal(fmt.Errorf("use -limit >= 0, -runs >= 1, and no positional arguments"))
	}
	var workers []int
	seenW := map[int]bool{}
	for _, s := range strings.Split(*workersFlag, ",") {
		w, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || w < 1 {
			fatal(fmt.Errorf("invalid worker count %q: use positive integers", s))
		}
		if !seenW[w] {
			workers = append(workers, w)
			seenW[w] = true
		}
	}
	selected := map[string]bool{}
	for _, s := range strings.Split(*modesFlag, ",") {
		s = strings.TrimSpace(s)
		if s == "all" {
			for _, m := range modeNames {
				selected[m] = true
			}
			continue
		}
		valid := false
		for _, m := range modeNames {
			if s == m {
				valid = true
			}
		}
		if !valid {
			fatal(fmt.Errorf("unknown mode %q; use -help for modes", s))
		}
		selected[s] = true
	}
	// Baseline first, then its cached-hash pass, regardless of flag ordering.
	var mods, scanRoots []string
	var rootFiles totals
	seenRoot := map[string]bool{}
	seenMod := map[string]bool{}
	for _, root := range strings.Split(*roots, ";") {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			fatal(err)
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			fmt.Printf("root unavailable: %s: %v\n", abs, err)
			continue
		}
		key := strings.ToLower(abs)
		if seenRoot[key] {
			continue
		}
		seenRoot[key] = true
		scanRoots = append(scanRoots, abs)
		for _, e := range entries {
			if e.Type().IsRegular() {
				info, err := e.Info()
				if err != nil {
					fatal(err)
				}
				rootFiles.file(uint64(info.Size()), uint64(info.ModTime().UnixNano()))
			}
			if !e.IsDir() || skipDir(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				fatal(err)
			}
			if isReparse(info) {
				continue
			}
			p := filepath.Join(abs, e.Name())
			key := strings.ToLower(p)
			if !seenMod[key] {
				mods = append(mods, p)
				seenMod[key] = true
			}
		}
	}
	if *limit > 0 && len(mods) > *limit {
		mods = mods[:*limit]
	}
	fmt.Printf("mods=%d GOMAXPROCS=%d runs=%d\n", len(mods), runtime.GOMAXPROCS(0), *runs)
	onlyRootModes := true
	for mode := range selected {
		if mode != "usn" && mode != "fsscan-walk" && mode != "fsscan-size" {
			onlyRootModes = false
		}
	}
	if len(mods) == 0 && !(onlyRootModes && (len(scanRoots) > 0 || selected["usn"])) {
		fatal(fmt.Errorf("no mods found; set -roots to existing parent folders"))
	}
	if selected["open-cold"] {
		if err := runOpenCold(mods, workers, *seed, *groupMods, *skipMods, *rewarm, *ext); err != nil {
			fatal(err)
		}
		delete(selected, "open-cold")
		if len(selected) == 0 {
			return
		}
	}
	fmt.Println("MB = decimal MB; checksum = sum(size + UnixNano(mtime)) modulo 2^64; dirs include mod roots.")
	fmt.Println("scan dirs use captured baseline; tex/about expose no filesystem totals (n/a). dirstat checksum sums directory mtimes only.")
	fmt.Println("scan/tex/about modes skip mods containing reparse points, avoiding links their internal helpers could follow.")
	if selected["fsscan-walk"] || selected["fsscan-size"] {
		fmt.Printf("fsscan modes scan each of %d complete roots once; -limit applies only to mod-based modes.\n", len(scanRoots))
		fmt.Println("fsscan-walk skips the same directory names as dirqueue-par; dirs are n/a. fsscan-size is unfiltered; dirs/checksum/skipped are n/a.")
	}
	fmt.Printf("%-19s %7s %12s %12s %9s %10s %12s %20s %7s %7s\n", "mode", "workers", "run", "wall_ms", "dirs", "files", "MB", "checksum", "errors", "skipped")
	snapshots := make([]snapshot, len(mods))
	captured := false
	var baseline totals
	var dirs []string
	ensureBaseline := func() {
		if captured {
			return
		}
		start := time.Now()
		for i, mod := range mods {
			baseline.add(walk(mod, &snapshots[i]))
			dirs = append(dirs, snapshots[i].dirs...)
		}
		captured = true
		fmt.Printf("baseline preparation (untimed prerequisite): %.3f ms, errors=%d\n", float64(time.Since(start))/float64(time.Millisecond), baseline.errors)
	}
	prev := make([]map[string]scan.File, len(mods))
	prevReady := false
	currentFiles := make([][]scan.File, len(mods))
	savePrevious := func() {
		for i, files := range currentFiles {
			next := make(map[string]scan.File, len(files))
			for _, f := range files {
				next[strings.ToLower(f.Rel)] = f
			}
			prev[i] = next
			currentFiles[i] = nil
		}
	}
	compute := func(w int, reuse bool) totals {
		return parallel(len(mods), w, func(i int) totals {
			if snapshots[i].links {
				return totals{skipped: 1}
			}
			var old map[string]scan.File
			if reuse {
				old = prev[i]
			}
			fp, err := scan.ComputeSkip(mods[i], old, nil)
			t := totals{dirs: snapshots[i].totals.dirs, hashed: fp.Hashed, reused: fp.Reused}
			t.fail(err)
			currentFiles[i] = fp.Files
			for _, f := range fp.Files {
				t.file(uint64(f.Size), uint64(f.MTime))
			}
			return t
		})
	}
	if *prepareBaseline && (*cpuProfile != "" || *memProfile != "") {
		ensureBaseline()
	}
	var cpuFile *os.File
	if *cpuProfile != "" {
		var err error
		cpuFile, err = os.Create(*cpuProfile)
		if err != nil {
			fatal(err)
		}
		if err := pprof.StartCPUProfile(cpuFile); err != nil {
			cpuFile.Close()
			fatal(err)
		}
	}
	defer func() {
		if cpuFile != nil {
			pprof.StopCPUProfile()
			if err := cpuFile.Close(); err != nil {
				fatal(err)
			}
		}
		if *memProfile != "" {
			runtime.GC()
			f, err := os.Create(*memProfile)
			if err != nil {
				fatal(err)
			}
			err = pprof.WriteHeapProfile(f)
			closeErr := f.Close()
			if err != nil {
				fatal(err)
			}
			if closeErr != nil {
				fatal(closeErr)
			}
		}
	}()
	for _, mode := range modeNames {
		if !selected[mode] {
			continue
		}
		if mode == "usn" {
			queryUSN(mods)
			continue
		}
		if mode != "walkdir" && (*prepareBaseline || mode == "dirstat" || mode == "tex-fingerprint" || mode == "about" || strings.HasPrefix(mode, "scan-compute")) {
			ensureBaseline()
		}
		if mode == "scan-compute-prev" && !prevReady {
			start := time.Now()
			t := compute(1, false)
			savePrevious()
			prevReady = true
			fmt.Printf("scan-compute-prev seed (untimed): %.3f ms, errors=%d, skipped=%d\n", float64(time.Since(start))/float64(time.Millisecond), t.errors, t.skipped)
		}
		counts := workers
		if mode == "walkdir" {
			counts = []int{1}
		}
		for _, w := range counts {
			elapsed := make([]float64, 0, *runs)
			printRow := func(run string, ms float64, t totals) {
				d, f, b, c := fmt.Sprint(t.dirs), fmt.Sprint(t.files), fmt.Sprintf("%.3f", float64(t.bytes)/1e6), fmt.Sprintf("%016x", t.checksum)
				if mode == "tex-fingerprint" || mode == "about" {
					d, f, b, c = "n/a", "n/a", "n/a", "n/a"
				}
				skipped := fmt.Sprint(t.skipped)
				if mode == "fsscan-walk" || mode == "fsscan-size" {
					d = "n/a"
				}
				if mode == "fsscan-size" {
					c, skipped = "n/a", "n/a"
				}
				fmt.Printf("%-19s %7d %12s %12.3f %9s %10s %12s %20s %7d %7s\n", mode, w, run, ms, d, f, b, c, t.errors, skipped)
			}
			var last totals
			stable := true
			for r := 0; r < *runs; r++ {
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				start := time.Now()
				var t totals
				switch mode {
				case "walkdir":
					for i, mod := range mods {
						if !captured {
							t.add(walk(mod, &snapshots[i]))
						} else {
							t.add(walk(mod, nil))
						}
					}
				case "walkdir-par":
					t = parallel(len(mods), w, func(i int) totals { return walk(mods[i], nil) })
				case "dirqueue-par":
					t = dirQueue(mods, w)
				case "fsscan-walk":
					for _, root := range scanRoots {
						tree, err := fsscan.Walk(context.Background(), root, fsscan.Options{
							Workers: w,
							SkipDir: func(rel, name string) bool { return skipDir(name) },
						})
						t.fail(err)
						if err == nil {
							t.skipped += tree.Skipped
							for _, entry := range tree.Entries {
								t.file(uint64(entry.Size), uint64(entry.MTime))
							}
						}
					}
				case "fsscan-size":
					for _, root := range scanRoots {
						files, bytes, err := fsscan.Size(context.Background(), root, w)
						t.fail(err)
						t.files += uint64(files)
						t.bytes += uint64(bytes)
					}
				case "findex", "fileinfo-big":
					t = parallel(len(mods), w, func(i int) totals { return nativeWalk(mods[i], mode == "fileinfo-big") })
				case "scan-compute", "scan-compute-prev":
					t = compute(w, mode == "scan-compute-prev")
					prevReady = true
				case "tex-fingerprint":
					t = parallel(len(mods), w, func(i int) totals {
						if snapshots[i].links {
							return totals{skipped: 1}
						}
						sum, err := textures.Fingerprint(context.Background(), mods[i])
						t := totals{checksum: binary.LittleEndian.Uint64(sum[:8])}
						t.fail(err)
						return t
					})
				case "about":
					t = parallel(len(mods), w, func(i int) totals {
						if snapshots[i].links {
							return totals{skipped: 1}
						}
						d := scan.ReadDecl(mods[i], false)
						var t totals
						if d.ParseError != "" {
							t.fail(fmt.Errorf("%s: %s", mods[i], d.ParseError))
						}
						return t
					})
				case "dirstat":
					t = parallel(len(dirs), w, func(i int) totals {
						var t totals
						info, err := os.Stat(dirs[i])
						t.fail(err)
						if err == nil {
							t.dirs++
							t.checksum += uint64(info.ModTime().UnixNano())
						}
						return t
					})
				}
				ms := float64(time.Since(start)) / float64(time.Millisecond)
				runtime.ReadMemStats(&after)
				// Cache indexing is harness setup, outside the measured API call.
				if mode == "scan-compute" || mode == "scan-compute-prev" {
					savePrevious()
				}
				if mode == "walkdir" && !captured {
					captured = true
					baseline = t
					for _, s := range snapshots {
						dirs = append(dirs, s.dirs...)
					}
				}
				label := strconv.Itoa(r + 1)
				if r == 0 {
					label += "(first)"
				}
				printRow(label, ms, t)
				fmt.Printf("  alloc_MB=%.3f allocs=%d GCs=%d\n", float64(after.TotalAlloc-before.TotalAlloc)/1e6, after.Mallocs-before.Mallocs, after.NumGC-before.NumGC)
				elapsed = append(elapsed, ms)
				if t.firstError != "" {
					fmt.Printf("  first error: %s\n", t.firstError)
				}
				if mode == "scan-compute" || mode == "scan-compute-prev" {
					fmt.Printf("  hashed=%d reused=%d\n", t.hashed, t.reused)
				}
				comparable := mode == "walkdir" || mode == "walkdir-par" || mode == "dirqueue-par" || mode == "findex" || mode == "fileinfo-big" || strings.HasPrefix(mode, "scan-compute")
				if comparable && captured && t.skipped == 0 && (t.dirs != baseline.dirs || t.files != baseline.files || t.bytes != baseline.bytes || t.checksum != baseline.checksum) {
					fmt.Println("  MISMATCH against first walkdir: tree changed, errors, or enumeration differs")
				}
				if mode == "fsscan-walk" && captured && *limit == 0 && t.skipped == 0 && (t.files != baseline.files+rootFiles.files || t.bytes != baseline.bytes+rootFiles.bytes || t.checksum != baseline.checksum+rootFiles.checksum) {
					fmt.Println("  MISMATCH against walkdir plus root files: tree changed, errors, or enumeration differs")
				}
				if r > 0 && (t.dirs != last.dirs || t.files != last.files || t.bytes != last.bytes || t.checksum != last.checksum || t.errors != last.errors || t.skipped != last.skipped) {
					stable = false
				}
				last = t
			}
			if len(elapsed) > 1 {
				rest := elapsed[1:]
				sort.Float64s(rest)
				median := rest[len(rest)/2]
				if len(rest)%2 == 0 {
					median = (rest[len(rest)/2-1] + median) / 2
				}
				if stable {
					printRow("median-rest", median, last)
				} else {
					fmt.Printf("%-19s %7d %12s %12.3f (totals varied; see individual runs)\n", mode, w, "median-rest", median)
				}
			} else {
				fmt.Printf("%-19s %7d median-rest: n/a (one run)\n", mode, w)
			}
		}
	}
}
