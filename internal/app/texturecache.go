package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// 06 §2.13 / 08 §5.2: cache summaries, not conversion jobs. Validate texture metadata
// even without a scan: folder mtimes alone miss an overwrite of an existing PNG.
type textureSummaryEntry struct {
	Fingerprint [32]byte
	Options     textures.Options
	Owned       [32]byte
	Summary     *TextureMod
}

func textureSummaryKey(dir string) string { return store.PathKey(dir) }

func (a *App) textureSummaries(ctx context.Context, targets []TextureTarget, o textures.Options, progress func(*TextureMod), workers int) (*TextureReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	last, err := a.Store.TextureMods()
	if err != nil {
		return nil, err
	}
	cachePath := filepath.Join(a.Home, "cache", "textures", "summaries-v1.json")
	cache := map[string]textureSummaryEntry{}
	if b, err := os.ReadFile(cachePath); err == nil {
		if json.Unmarshal(b, &cache) != nil {
			cache = nil
		}
	}
	if cache == nil {
		cache = map[string]textureSummaryEntry{}
	}
	o = o.Normal()
	r := &TextureReport{Options: o, Tool: a.TextureTool(), DryRun: true, Mods: make([]*TextureMod, len(targets))}
	type result struct {
		index   int
		key     string
		entry   textureSummaryEntry
		changed bool
		err     error
	}
	jobs := make(chan int)
	results := make(chan result)
	workers = max(1, min(workers, runtime.GOMAXPROCS(0), len(targets)))
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				target := targets[i]
				dir := target.Dir
				if real, err := source.RealPath(dir); err == nil {
					dir = real
				}
				key := textureSummaryKey(dir)
				fingerprint, err := textures.Fingerprint(ctx, dir)
				if err != nil {
					select {
					case results <- result{err: err}:
					case <-ctx.Done():
					}
					continue
				}
				owned, err := a.Store.TextureFilesIn(dir)
				if err != nil {
					select {
					case results <- result{err: err}:
					case <-ctx.Done():
					}
					continue
				}
				ledger, _ := json.Marshal(struct {
					Files []store.TextureFile
					Last  store.TextureMod
				}{owned, last[target.Mod.ID]})
				stamp := sha256.Sum256(ledger)
				old, hit := cache[key]
				hit = hit && old.Summary != nil && old.Fingerprint == fingerprint && old.Options == o && old.Owned == stamp
				entry := old
				if !hit {
					// readTextureMod only uses immutable inputs, Home and database operations;
					// workers share no mutable App index or per-mod job state.
					t := a.readTextureMod(ctx, target.Mod, target.Ref, dir, o, false, last)
					t.set, t.rows = nil, nil
					if t.Plan != nil {
						t.Plan.Jobs, t.Plan.Fresh = nil, nil
					}
					entry = textureSummaryEntry{Fingerprint: fingerprint, Options: o, Owned: stamp, Summary: t}
				}
				t := *entry.Summary
				t.Mod, t.Ref, t.Dir = target.Mod, target.Ref, dir
				entry.Summary = &t
				select {
				case results <- result{index: i, key: key, entry: entry, changed: !hit}:
				case <-ctx.Done():
					return
				}
			}
		})
	}
	go func() {
		defer close(jobs)
		for i := range targets {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	// Apply callbacks serially; request cancellation still guards every publication (06 §3.1).
	changed := false
	var readErr error
	updates := map[string]textureSummaryEntry{}
	for res := range results {
		if res.err != nil {
			readErr = res.err
			continue
		}
		t := res.entry.Summary
		r.Mods[res.index] = t
		if t.Plan != nil {
			r.Est.Plus(t.Plan.Est)
		}
		if res.changed && t.Err == "" {
			updates[res.key] = res.entry
			changed = true
		}
		if ctx.Err() == nil && progress != nil {
			progress(t)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, readErr
	}
	if changed {
		for key, entry := range updates {
			cache[key] = entry
		}
		// One atomic write for the page, rather than 450 summary files per visit.
		writeTextureSummaries(ctx, cachePath, cache)
	}
	return r, nil
}

func writeTextureSummaries(ctx context.Context, path string, entries map[string]textureSummaryEntry) {
	if ctx.Err() != nil || os.MkdirAll(filepath.Dir(path), 0755) != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(path), "summary-*.tmp")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(entries)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil && ctx.Err() == nil {
		_ = os.Rename(f.Name(), path)
	}
}

// Kept modest for shared disks; first-read concurrency is measured in 07 §6.2.
const textureSummaryWorkers = 4
