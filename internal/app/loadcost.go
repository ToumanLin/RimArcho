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
	Game     loadcost.Game    `json:"game"`
	Mods     int              `json:"mods"`
	Measured int              `json:"measured"`
	Cached   int              `json:"cached"`
	Baseline LoadCostBaseline `json:"baseline"`
	Top      []TopMod         `json:"top"`
	PerMod   []ModCost        `json:"perMod"`
}

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
	type target struct {
		packageID string
		dir       string
		folders   []string
		top       TopMod
	}
	targets := make([]target, 0, len(mods))
	for _, m := range mods {
		dir := st.Ix.ModDir(m)
		if real, err := source.RealPath(dir); err == nil {
			dir = real
		}
		res := content.Resolve(dir, m.Decl().LoadFolders, st.GameMM(), func(pkg string) bool {
			return active[strings.TrimSuffix(strings.ToLower(pkg), "_steam")]
		})
		ref := st.Ix.Ref(m.ID)
		targets = append(targets, target{packageID: m.PackageID, dir: dir, folders: res.Folders, top: TopMod{Name: m.DisplayName(), Ref: ref, Page: m.ID}})
	}
	g := loadcost.Game16
	if st.GameMM() != "" && !env.VersionLess(st.GameMM(), "1.7") {
		g = loadcost.Game17
	}
	r := &LoadCost{Game: g, Mods: len(targets), PerMod: make([]ModCost, len(targets))}
	r.Baseline.RAM, r.Baseline.VRAM = loadcost.Baseline(g)
	cache := fsscan.OpenCache(filepath.Join(a.Home, "cache", "loadcost-v1.json"))
	measured := make([]loadcost.Mod, len(targets))
	top := make([]TopMod, len(targets))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	var firstErr error
	done := 0
	if progress != nil {
		progress(0, len(targets))
	}
	workers := 1
	if len(targets) > 0 {
		workers = min(8, fsscan.Workers(targets[0].dir))
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				t := targets[i]
				stamp, err := loadCostStamp(ctx, t.folders)
				var m loadcost.Mod
				hit := false
				if err == nil {
					hit = cache.Get(store.PathKey(t.dir), "mod", stamp, &m)
					if !hit {
						pngCache := filepath.Join(a.Home, "cache", "textures", fmt.Sprintf("%x.json", sha256.Sum256([]byte(store.PathKey(t.dir)))))
						m, err = loadcost.Measure(ctx, t.dir, t.folders, pngCache)
						if err == nil {
							cache.Put(store.PathKey(t.dir), "mod", stamp, m)
						}
					}
				}
				mu.Lock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					cancel()
					mu.Unlock()
					return
				}
				measured[i] = m
				report := loadcost.Estimate(g, []loadcost.Mod{m})
				r.PerMod[i] = ModCost{PackageID: t.packageID, Name: t.top.Name, Ref: t.top.Ref, Measure: m, Est: report}
				t.top.RAMHigh, t.top.VRAMHigh = report.RAMHigh, report.VRAMHigh
				top[i] = t.top
				if hit {
					r.Cached++
				} else {
					r.Measured++
				}
				done++
				if progress != nil {
					progress(done, len(targets))
				}
				mu.Unlock()
			}
		})
	}
send:
	for i := range targets {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break send
		}
	}
	close(jobs)
	wg.Wait()
	saveErr := cache.Save()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if saveErr != nil {
		return nil, fmt.Errorf("save load cost cache: %w", saveErr)
	}
	r.Report = loadcost.Estimate(g, measured)
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
				r.Top = append(r.Top, m)
				seenTop[m.Ref] = true
			}
		}
	}
	return r, nil
}

func loadCostStamp(ctx context.Context, folders []string) ([16]byte, error) {
	h := xxh3.New()
	h.WriteString("loadcost/v1")
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
