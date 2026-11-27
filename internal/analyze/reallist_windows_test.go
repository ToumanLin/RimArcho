package analyze_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/harmony"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// TestRealListCheck times the Mod check of a real list in place, phase by phase: a first
// check (empty store and Harmony fact cache), then two repeats (18 §2, "Mod check of a
// 720-Mod list"). Opt-in and read-only: it reads the Mods where they are and writes only to
// the test's temporary folder (set TMP under D:\RimArcho-test on the host; real lists
// belong in the VM unless the player agrees, 17 §4).
//
//	RIMARCHO_REALCHECK_LIST      file of "position|kind|folder[|link target]" lines, kind
//	                             "official" for Core and DLCs (load order = position)
//	RIMARCHO_REALCHECK_INACTIVE  a folder whose other subfolders are disabled Mods (optional)
//	RIMARCHO_REALCHECK_MM        game version, default 1.7
//	RIMARCHO_REALCHECK_BACKGROUND=1  as an automatic check: two threads (app.SetAnalyzeBackground)
//	RIMARCHO_REALCHECK_OUT       folder for CPU profiles and the results as JSON (optional)
//
// Like the `archo analyze` child it sets app.AnalyzeMemoryLimit. The store has an instance
// row per Mod, so stored facts are kept between the runs as in a real data folder.
func TestRealListCheck(t *testing.T) {
	list := os.Getenv("RIMARCHO_REALCHECK_LIST")
	if list == "" {
		t.Skip("set RIMARCHO_REALCHECK_LIST to time the check of a real list (read-only)")
	}
	mm := os.Getenv("RIMARCHO_REALCHECK_MM")
	if mm == "" {
		mm = "1.7"
	}
	mods := realMods(t, list, os.Getenv("RIMARCHO_REALCHECK_INACTIVE"))
	defer debug.SetMemoryLimit(debug.SetMemoryLimit(app.AnalyzeMemoryLimit))
	workers := 0
	if os.Getenv("RIMARCHO_REALCHECK_BACKGROUND") != "" {
		defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(app.BackgroundWorkers))
		workers = app.BackgroundWorkers
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.MM = mm
	for _, m := range mods {
		if _, err := st.DB.Exec(`INSERT INTO instance(id,game_id,lineage_id,source_id,rel_path,status,first_seen,last_seen) VALUES(?,'g',?,'s',?,'live',0,0)`, m.Instance, m.Lineage, m.Dir); err != nil {
			t.Fatal(err)
		}
	}
	cache := &dirCache{dir: filepath.Join(t.TempDir(), "harmony")}
	out := os.Getenv("RIMARCHO_REALCHECK_OUT")
	for _, name := range []string{"first", "repeat", "repeat2"} {
		var phases []string
		t0 := time.Now()
		last := t0
		in := analyze.Input{Mods: mods, MM: mm, Workers: workers, Cached: st.CachedDefs, Assemblies: cache,
			Progress: func(l string) {
				if strings.Contains(l, "/") && strings.Contains(l, "operations") || strings.HasPrefix(l, "defs: ") && strings.Contains(l, "/") {
					return // the per-500-files and per-1000-operations lines
				}
				now := time.Now()
				phases = append(phases, fmt.Sprintf("%6d ms (+%5d) %s", now.Sub(t0).Milliseconds(), now.Sub(last).Milliseconds(), l))
				last = now
			}}
		stop := profile(t, out, name)
		c0 := cpuTime()
		o, err := analyze.Run(in)
		if err != nil {
			t.Fatal(err)
		}
		runWall, runCPU := time.Since(t0), cpuTime()-c0
		s0, sc := time.Now(), cpuTime()
		if _, err := st.SaveAnalysisContext(context.Background(), "g", "set", "run", mm, nil, o, t0, time.Now()); err != nil {
			t.Fatal(err)
		}
		st.TruncateWAL()
		saveWall, saveCPU := time.Since(s0), cpuTime()-sc
		stop()
		hits, misses := cache.counts()
		t.Logf("%s: check %v (CPU %v), save %v (CPU %v); Defs %d ms, Patch %d ms, Harmony %d ms (DLL facts: %d cached, %d read); inactive Mods cached %d; peak heap %d MB",
			name, runWall.Round(time.Millisecond), runCPU.Round(time.Millisecond), saveWall.Round(time.Millisecond), saveCPU.Round(time.Millisecond),
			o.Stats.DefsMs, o.Stats.PatchMs, o.Stats.HarmonyMs, hits, misses, o.Stats.CachedMods, o.Stats.PeakHeapMB)
		for _, p := range phases {
			t.Log("  " + p)
		}
		if out != "" {
			b, _ := json.Marshal(struct {
				Findings []*analyze.Finding
				Patches  []*analyze.PatchFact
			}{o.Findings, o.Patches})
			os.WriteFile(filepath.Join(out, name+".json"), b, 0o644)
		}
	}
}

// realMods reads the list file and the disabled Mods' folders.
func realMods(t *testing.T, list, inactive string) []*analyze.Mod {
	f, err := os.Open(list)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var mods []*analyze.Mod
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p := strings.Split(sc.Text(), "|")
		if len(p) < 3 {
			continue
		}
		pos, err := strconv.Atoi(p[0])
		if err != nil {
			t.Fatalf("%s: %q", list, sc.Text())
		}
		dir := p[2]
		if len(p) > 3 && p[3] != "" {
			dir = p[3] // a link's target
		}
		dir = filepath.Clean(dir)
		seen[strings.ToLower(dir)] = true
		mods = append(mods, realMod(dir, p[1] == "official", pos+1))
	}
	if inactive != "" {
		es, err := os.ReadDir(inactive)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range es {
			if d := filepath.Join(inactive, e.Name()); e.IsDir() && !seen[strings.ToLower(d)] {
				mods = append(mods, realMod(d, false, 0))
			}
		}
	}
	return mods
}

func realMod(dir string, official bool, pos int) *analyze.Mod {
	d := scan.ReadDecl(dir, official)
	h := sha256.Sum256([]byte(dir))
	name := d.Name
	if official {
		name = filepath.Base(dir)
	}
	return &analyze.Mod{Instance: fmt.Sprintf("%x", h[:8]), Lineage: d.PackageID, SnapID: 1, SnapHash: h[:], Dir: dir,
		Pkg: strings.TrimSuffix(strings.ToLower(d.PackageID), "_steam"), Name: name, Decl: d, Pos: pos}
}

// dirCache is the app's Harmony fact cache without pruning (app/asmcache.go), counting.
type dirCache struct {
	dir          string
	mu           sync.Mutex
	hits, misses int
}

func (c *dirCache) Get(key string) (*harmony.Assembly, bool) {
	var a harmony.Assembly
	b, err := os.ReadFile(filepath.Join(c.dir, key+".json"))
	ok := err == nil && json.Unmarshal(b, &a) == nil
	c.mu.Lock()
	if ok {
		c.hits++
	} else {
		c.misses++
	}
	c.mu.Unlock()
	return &a, ok
}

func (c *dirCache) Put(key string, a *harmony.Assembly) {
	b, _ := json.Marshal(a)
	os.MkdirAll(c.dir, 0o755)
	os.WriteFile(filepath.Join(c.dir, key+".json"), b, 0o644)
}

func (c *dirCache) counts() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h, m := c.hits, c.misses
	c.hits, c.misses = 0, 0
	return h, m
}

func profile(t *testing.T, out, name string) func() {
	if out == "" {
		return func() {}
	}
	f, err := os.Create(filepath.Join(out, name+".cpu.prof"))
	if err != nil {
		t.Fatal(err)
	}
	pprof.StartCPUProfile(f)
	return func() { pprof.StopCPUProfile(); f.Close() }
}

// cpuTime is the process's user and kernel time.
func cpuTime() time.Duration {
	h, _ := syscall.GetCurrentProcess()
	var c, e, k, u syscall.Filetime
	syscall.GetProcessTimes(h, &c, &e, &k, &u)
	return time.Duration(k.Nanoseconds() + u.Nanoseconds())
}
