package app

import (
	"context"
	"fmt"
	"slices"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// SourceProjects visits cached projects first, then decompiles missing projects
// on demand. It shares DLL identities and durable publication with CLI/MCP, but
// has no dispatch budget. visit and finished run synchronously on this goroutine.
func (a *App) SourceProjects(ctx context.Context, st *LoadoutState, mods []*store.Mod, game bool, visit func(*SourceDLL) error, finished func(string), note func(string)) (int, error) {
	release, err := a.sourceAccess(ctx, false, true)
	if err != nil {
		return 0, err
	}
	defer release()
	var projects []*SourceDLL
	byHash := map[string]*SourceDLL{}
	remaining := map[string]int{}
	add := func(d *SourceDLL) {
		key := d.Hash + "|" + d.GameBuild
		if first := byHash[key]; first != nil {
			if slices.Contains(first.Shared, d.Instance) {
				return
			}
			remaining[d.Instance]++
			first.Shared = append(first.Shared, d.Instance)
		} else {
			remaining[d.Instance]++
			d.Shared = []string{d.Instance}
			byHash[key] = d
			projects = append(projects, d)
		}
	}
	for _, m := range mods {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		ds, err := a.SourceDLLs(st, m)
		if err != nil {
			return 0, err
		}
		for _, d := range ds {
			add(d)
		}
	}
	if game {
		d, err := a.GameSourceDLL(st)
		if err != nil {
			note(err.Error())
		} else {
			add(d)
		}
	}
	for _, m := range mods {
		if remaining[m.ID] == 0 && finished != nil {
			finished(m.ID)
		}
	}
	decompiled := 0
	// Partition once: a project published by this run must not be visited twice.
	var cached, missing []*SourceDLL
	for _, d := range projects {
		if d.Cached {
			cached = append(cached, d)
		} else {
			missing = append(missing, d)
		}
	}
	for _, batch := range [][]*SourceDLL{cached, missing} {
		for _, d := range batch {
			if err := ctx.Err(); err != nil {
				return decompiled, err
			}
			wasCached := d.Cached
			if err := a.DecompileDLL(ctx, st, d); err != nil {
				if ctx.Err() != nil {
					return decompiled, ctx.Err()
				}
				note(fmt.Sprintf("%s: %v", d.File, err))
			} else {
				if !wasCached {
					decompiled++
				}
				for _, id := range d.Shared {
					copy := *d
					copy.Instance = id
					if err := a.recordSourceUse(&copy, "search-hit", nil); err != nil {
						return decompiled, err
					}
				}
				if err := visit(d); err != nil {
					return decompiled, err
				}
			}
			for _, id := range d.Shared {
				remaining[id]--
				if remaining[id] == 0 && finished != nil {
					finished(id)
				}
			}
		}
	}
	return decompiled, ctx.Err()
}
