package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/fsscan"
	"github.com/ToumanLin/RimArcho/internal/loadcost"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/zeebo/xxh3"
)

type LoadCostBaseline struct {
	RAM  int64 `json:"ram"`
	VRAM int64 `json:"vram"`
}

type TopMod struct {
	Name     string `json:"name"`
	Ref      string `json:"ref"`
	Page     string `json:"page"`
	RAMHigh  int64  `json:"ramHigh"`
	VRAMHigh int64  `json:"vramHigh"`
}

// ModCost keeps one mod's raw measurements and mod-only estimate in load order.
type ModCost struct {
	PackageID string          `json:"packageId"`
	Name      string          `json:"name"`
	Ref       string          `json:"ref"`
	Measure   loadcost.Mod    `json:"measure"`
	Est       loadcost.Report `json:"est"`
}

type LoadCost struct {
	loadcost.Report
	LoadingRAMLow  int64            `json:"loadingRAMLow"`
	LoadingRAMHigh int64            `json:"loadingRAMHigh"`
	Capacity       MemoryAssessment `json:"capacity"`
	Game           loadcost.Game    `json:"game"`
	Mods           int              `json:"mods"`
	Measured       int              `json:"measured"`
	Cached         int              `json:"cached"`
	Baseline       LoadCostBaseline `json:"baseline"`
	Top            []TopMod         `json:"top"`
	PerMod         []ModCost        `json:"perMod"`
}

// LoadFit is a LoadCost on one machine: baseline-included ranges after the hardware
// adjustments, 1.6 VRAM the card cannot hold (loadcost.Spill) and 1.7 textures above
// the card's upload watermark (loadcost.Cap17).
type LoadFit struct {
	// WSLow / WSHigh: the expected working set a minute after the main menu (what Task
	// Manager shows; loadcost.WorkingSet), the held VRAM included. The other RAM ranges
	// are private bytes (commit).
	WSLow, WSHigh           int64
	RAMLow, RAMHigh         int64 // resident
	LoadingLow, LoadingHigh int64
	VRAMLow, VRAMHigh       int64
	// HeldLow / HeldHigh: 1.6 VRAM held in system memory, included in the RAM ranges.
	HeldLow, HeldHigh int64
	// OnDemand: about how much 1.7 texture VRAM goes beyond what the game keeps on this
	// card (the low estimate's cut, near the BC7 payload; the high one's when only it is
	// cut). It stays in the disk cache and loads when drawn; left out of the ranges above.
	OnDemand int64
}

// FitTo adjusts the hardware-independent estimate to a card with dedicated bytes of
// VRAM (<= 0: unknown, no dedicated memory).
func (r *LoadCost) FitTo(dedicated int64) LoadFit {
	cutRAMLow, cutRAMHigh, cutVRAMLow, cutVRAMHigh := loadcost.Cap17(r.Game, r.Report, dedicated)
	f := LoadFit{
		RAMLow: r.RAMLow - cutRAMLow + r.Baseline.RAM, RAMHigh: r.RAMHigh - cutRAMHigh + r.Baseline.RAM,
		LoadingLow: r.LoadingRAMLow - cutRAMLow + r.Baseline.RAM, LoadingHigh: r.LoadingRAMHigh - cutRAMHigh + r.Baseline.RAM,
		VRAMLow: r.VRAMLow - cutVRAMLow + r.Baseline.VRAM, VRAMHigh: r.VRAMHigh - cutVRAMHigh + r.Baseline.VRAM,
		OnDemand: cutVRAMLow,
	}
	if f.OnDemand == 0 {
		f.OnDemand = cutVRAMHigh
	}
	f.HeldLow, f.HeldHigh = loadcost.Spill(r.Game, f.VRAMLow, f.VRAMHigh, dedicated)
	f.RAMLow += f.HeldLow
	f.RAMHigh += f.HeldHigh
	f.LoadingLow += f.HeldLow
	f.LoadingHigh += f.HeldHigh
	f.WSLow, f.WSHigh = loadcost.WorkingSet(r.Game, r.Report, r.Mods)
	f.WSLow += f.HeldLow
	f.WSHigh += f.HeldHigh
	// The private working set is part of the private bytes: never above the resident range.
	f.WSLow, f.WSHigh = min(f.WSLow, f.RAMLow), min(f.WSHigh, f.RAMHigh)
	return f
}

// Fit is FitTo on the hardware Capacity was assessed with.
func (r *LoadCost) Fit() LoadFit { return r.FitTo(r.Capacity.Hardware.VRAM()) }

// PlanLoadCost measures the selected plan, using its own enabled entries.
// Progress callbacks are serialized, including the initial (0, total) callback.
func (a *App) PlanLoadCost(ctx context.Context, planID int64, progress func(done, total int)) (*LoadCost, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	p := st.Plan
	if planID != 0 && planID != p.ID {
		p, err = a.Store.PlanByID(planID)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, fmt.Errorf("no plan %d", planID)
		}
	}
	ps, err := a.PlanState(st, p)
	if err != nil {
		return nil, err
	}
	mm := ps.GameMM()
	if mm == "" {
		return nil, fmt.Errorf("cannot estimate memory: game version is unknown for %s; rescan the selected game (archo scan) and retry", st.Ix.Env.Name)
	}
	targets, err := a.loadCostTargets(st, ps, mm)
	if err != nil {
		return nil, err
	}
	g := loadcost.Game16
	if !env.VersionLess(mm, "1.7") {
		g = loadcost.Game17
	}
	r := &LoadCost{Game: g, Mods: len(targets), PerMod: make([]ModCost, len(targets))}
	r.Baseline.RAM, r.Baseline.VRAM = loadcost.Baseline(g)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	run := &loadCostRun{
		a: a, g: g, r: r, targets: targets, progress: progress, cancel: cancel,
		cache:    fsscan.OpenCache(filepath.Join(a.Home, "cache", "loadcost-v1.json")),
		measured: make([]loadcost.Mod, len(targets)),
		top:      make([]TopMod, len(targets)),
	}
	saveErr := run.measureAll(ctx)
	if run.firstErr != nil {
		return nil, run.firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if saveErr != nil {
		return nil, fmt.Errorf("save load cost cache: %w", saveErr)
	}
	r.Report = loadcost.Estimate(g, run.measured)
	r.LoadingRAMLow, r.LoadingRAMHigh = loadcost.LoadingBudget(g, r.Report, run.measured)
	r.Capacity = AssessMemory(a.measuredMemory(), r)
	r.Capacity.LastRun = a.lastRunMemory(ps)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := a.saveMemoryEstimate(ps, r); err != nil {
		return nil, err
	}
	r.Top = topLoadCostMods(run.top)
	return r, nil
}

// loadCostTarget is one Mod to measure: its folders as the plan loads them.
type loadCostTarget struct {
	packageID string
	dir       string
	folders   []string
	top       TopMod
}

// loadCostTargets lists the plan's enabled, non-official Mods once each, with the folders
// they load under game version mm.
func (a *App) loadCostTargets(st *LoadoutState, ps *LoadoutState, mm string) ([]loadCostTarget, error) {
	var mods []*store.Mod
	active, seen := map[string]bool{}, map[string]bool{}
	for _, e := range ps.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			active[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = true
			if !seen[m.ID] && m.Kind != source.Official && m.TombstoneAt.IsZero() {
				mods = append(mods, m)
				seen[m.ID] = true
			}
		}
	}
	if err := a.Store.LoadDecls(mods); err != nil {
		return nil, err
	}
	targets := make([]loadCostTarget, 0, len(mods))
	for _, m := range mods {
		dir := st.Ix.ModDir(m)
		if real, err := source.RealPath(dir); err == nil {
			dir = real
		}
		res := content.Resolve(dir, m.Decl().LoadFolders, mm, func(pkg string) bool {
			return active[strings.TrimSuffix(strings.ToLower(pkg), "_steam")]
		})
		ref := st.Ix.Ref(m.ID)
		targets = append(targets, loadCostTarget{packageID: m.PackageID, dir: dir, folders: res.Folders, top: TopMod{Name: m.DisplayName(), Ref: ref, Page: m.ID}})
	}
	return targets, nil
}

// loadCostRun is one measurement of a plan's targets by parallel workers; mu guards the
// results, the counters and the progress callback.
type loadCostRun struct {
	a        *App
	g        loadcost.Game
	r        *LoadCost
	targets  []loadCostTarget
	progress func(done, total int)
	cancel   context.CancelFunc
	cache    *fsscan.Cache
	measured []loadcost.Mod
	top      []TopMod

	mu       sync.Mutex
	firstErr error
	done     int
}

// measureAll measures every target with up to eight workers, stopping at the first error
// or when ctx ends, and returns the error saving the cache.
func (run *loadCostRun) measureAll(ctx context.Context) error {
	if run.progress != nil {
		run.progress(0, len(run.targets))
	}
	workers := 1
	if len(run.targets) > 0 {
		workers = min(8, fsscan.Workers(run.targets[0].dir))
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() { run.work(ctx, jobs) })
	}
send:
	for i := range run.targets {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break send
		}
	}
	close(jobs)
	wg.Wait()
	return run.cache.Save()
}

// work measures the targets whose indexes arrive on jobs until jobs closes, ctx ends or a
// measurement fails.
func (run *loadCostRun) work(ctx context.Context, jobs <-chan int) {
	for i := range jobs {
		if ctx.Err() != nil {
			return
		}
		t := run.targets[i]
		m, hit, err := run.measure(ctx, t)
		run.mu.Lock()
		if err != nil {
			if run.firstErr == nil {
				run.firstErr = err
			}
			run.cancel()
			run.mu.Unlock()
			return
		}
		run.measured[i] = m
		report := loadcost.Estimate(run.g, []loadcost.Mod{m})
		run.r.PerMod[i] = ModCost{PackageID: t.packageID, Name: t.top.Name, Ref: t.top.Ref, Measure: m, Est: report}
		t.top.RAMHigh, t.top.VRAMHigh = report.RAMHigh, report.VRAMHigh
		run.top[i] = t.top
		if hit {
			run.r.Cached++
		} else {
			run.r.Measured++
		}
		run.done++
		if run.progress != nil {
			run.progress(run.done, len(run.targets))
		}
		run.mu.Unlock()
	}
}

// measure returns a target's measurement from the cache (hit) or by measuring its files.
func (run *loadCostRun) measure(ctx context.Context, t loadCostTarget) (loadcost.Mod, bool, error) {
	stamp, err := loadCostStamp(ctx, t.folders)
	var m loadcost.Mod
	hit := false
	if err == nil {
		hit = run.cache.Get(store.PathKey(t.dir), "mod", stamp, &m)
		if !hit {
			pngCache := filepath.Join(run.a.Home, "cache", "textures", fmt.Sprintf("%x.json", sha256.Sum256([]byte(store.PathKey(t.dir)))))
			m, err = loadcost.Measure(ctx, t.dir, t.folders, pngCache)
			if err == nil {
				run.cache.Put(store.PathKey(t.dir), "mod", stamp, m)
			}
		}
	}
	return m, hit, err
}

// topLoadCostMods picks the five Mods with the most VRAM, then the five with the most RAM,
// each once. It sorts top in place.
func topLoadCostMods(top []TopMod) []TopMod {
	var out []TopMod
	seenTop := map[string]bool{}
	for _, ram := range []bool{false, true} {
		slices.SortStableFunc(top, func(a, b TopMod) int {
			x, y := a.VRAMHigh, b.VRAMHigh
			if ram {
				x, y = a.RAMHigh, b.RAMHigh
			}
			if x > y {
				return -1
			}
			if x < y {
				return 1
			}
			return strings.Compare(a.Ref, b.Ref)
		})
		for _, m := range top[:min(5, len(top))] {
			if !seenTop[m.Ref] {
				out = append(out, m)
				seenTop[m.Ref] = true
			}
		}
	}
	return out
}

func loadCostStamp(ctx context.Context, folders []string) ([16]byte, error) {
	h := xxh3.New()
	h.WriteString("loadcost/v2")
	for _, folder := range folders {
		h.WriteString("\x00" + folder + "\x00")
		t, err := fsscan.Walk(ctx, folder, fsscan.Options{
			Workers: 4,
			SkipDir: func(rel, name string) bool {
				switch strings.ToLower(name) {
				case ".git", ".svn", ".rimarcho":
					return true
				}
				parts := strings.Split(strings.ToLower(rel), "/")
				return !loadCostDir(parts[0]) || (parts[0] == "textures" && strings.EqualFold(name, "source"))
			},
			Keep: func(rel, name string) bool {
				dir, _, ok := strings.Cut(strings.ToLower(rel), "/")
				return ok && loadCostDir(dir)
			},
		})
		if errors.Is(err, fs.ErrNotExist) {
			h.WriteString("missing") // LoadFolders.xml may name a folder the mod does not ship
			continue
		}
		if err != nil {
			return [16]byte{}, err
		}
		stamp := t.Hash()
		h.Write(stamp[:])
	}
	return h.Sum128().Bytes(), nil
}

func loadCostDir(name string) bool {
	switch name {
	case "textures", "sounds", "defs", "patches", "languages", "assemblies":
		return true
	}
	return false
}
