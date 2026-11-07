package gui

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/menubg"
	"github.com/ToumanLin/RimArcho/internal/view"
)

type backgroundResult struct {
	items []app.Background
	done  bool
}

// Protected by API.mu. This is incidental background work, outside Tasks (16 §3.1).
type backgroundReader struct {
	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup
	closed  bool
	key     string
	active  string
	jobs    map[string]*backgroundResult
}

// Capture immutable inputs only. File discovery, ZIP reads, DDS decoding and JPEG
// encoding all run outside the API lock; an environment switch rejects stale results.
func (s *API) prepareBackgrounds(fresh bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.a.Env()
	if err != nil || s.bgRead.closed {
		return
	}
	base := e.ID + "|" + menubg.InventoryStamp(e.GameRoot) + "|" + view.Lang()
	if !fresh && s.bgRead.key == base {
		return
	}
	var targets []app.BackgroundTarget
	if ix, err := s.a.Index(); err == nil {
		targets = s.a.BackgroundTargets(ix)
	}
	key := base + fmt.Sprintf("|%v", targets)
	if s.bgRead.jobs == nil {
		s.bgRead.jobs = map[string]*backgroundResult{}
		s.bgRead.ctx, s.bgRead.cancel = context.WithCancel(context.Background())
	}
	job := s.bgRead.jobs[key]
	if s.bgRead.key != base || s.bgsEnv != e.ID {
		s.bgs = []app.Background{} // plain until the current game's cache is ready
	}
	previous := slices.Clone(s.bgs)
	s.bgRead.key, s.bgRead.active, s.bgsEnv = base, key, e.ID
	if job != nil {
		if job.done {
			s.bgs = job.items
		}
		if !fresh || !job.done {
			return
		}
	}
	job = &backgroundResult{}
	s.bgRead.jobs[key] = job
	ctx, home, game, lang := s.bgRead.ctx, s.a.Home, e.GameRoot, view.Lang()
	s.bgRead.workers.Add(1)
	go func() {
		defer s.bgRead.workers.Done()
		publish := func(items []app.Background, done bool) {
			s.mu.Lock()
			job.items, job.done = items, done
			current, envErr := s.a.Env()
			active := !s.bgRead.closed && ctx.Err() == nil && envErr == nil && current.ID == e.ID && current.GameRoot == game && s.bgRead.active == key
			changed := !slices.Equal(s.bgs, items)
			if active {
				s.bgs = items
			}
			s.mu.Unlock()
			if active && changed {
				s.send("backgrounds", nil)
			}
		}
		cache := filepath.Join(home, "cache", "backgrounds")
		cached, _ := menubg.Cached(ctx, game, cache, lang)
		if len(cached) > 0 {
			// Show shared art before inspecting another installation or reading mods.
			publish(mergeMenuBackgrounds(cached, previous), false)
		}
		var games []string
		if environments, err := s.a.Store.Envs(); err == nil {
			for _, other := range environments {
				games = append(games, other.GameRoot)
			}
		}
		menu, err := menubg.PrepareFrom(ctx, game, cache, lang, games)
		if err != nil && ctx.Err() == nil {
			log.Printf("menu backgrounds: %v", err)
		}
		// Cached/extracted official art needs no Preview.png filesystem discovery.
		fallbackTargets := slices.DeleteFunc(slices.Clone(targets), func(t app.BackgroundTarget) bool {
			if !t.Official {
				return false
			}
			for _, it := range menu {
				if strings.EqualFold(it.Key, "game:"+t.PackageID) {
					return true
				}
			}
			return false
		})
		publish(mergeMenuBackgrounds(menu, app.CollectBackgrounds(fallbackTargets)), true)
	}()
}

// Prefer extracted art per package, retaining previews for individual failures.
func mergeMenuBackgrounds(menu []menubg.Item, fallbacks []app.Background) []app.Background {
	items := make([]app.Background, 0, len(menu)+len(fallbacks))
	seen := map[string]bool{}
	for _, b := range menu {
		items = append(items, app.Background{Key: b.Key, Label: b.Label, Src: b.Full, Thumb: b.Thumb})
		seen[b.Key] = true
	}
	for _, b := range fallbacks {
		if !seen[b.Key] {
			items = append(items, b)
		}
	}
	slices.SortStableFunc(items, func(a, b app.Background) int {
		if a.Key == app.BgDefault && b.Key != app.BgDefault {
			return -1
		}
		if b.Key == app.BgDefault && a.Key != app.BgDefault {
			return 1
		}
		if a.Group == "" && b.Group != "" {
			return -1
		}
		if b.Group == "" && a.Group != "" {
			return 1
		}
		return 0
	})
	return items
}
