package gui

import (
	"runtime/debug"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// The window process's memory (07 §8.3). Go does not collect on its own while the window
// sits idle, so what a page built stays resident after the page is gone; the window returns
// it a moment after heavy work, and forgets a page's caches when the player leaves the page.

// MemoryLimit is the window process's Go soft memory cap. The live heap is 6–10 MB; the cap
// keeps a page's build (Mod 库存) from growing the heap far past that: the peak there went
// from 97 to 78 MB. Releasing after heavy work does the rest (measured in 07 §6).
const MemoryLimit = 32 << 20

// tuneProcess applies the window's memory settings: the soft cap, and a 256 KiB SQLite page
// cache on the one connection, as in the TUI (the default 2 MB stays resident in modernc's
// allocator).
func tuneProcess(a *app.App) {
	debug.SetMemoryLimit(MemoryLimit)
	a.Store.DB.Exec("PRAGMA cache_size=-256")
}

// releaseAfter is how long the window waits after heavy work before returning freed memory
// to the OS; more work in the meantime starts the wait again.
const releaseAfter = 2 * time.Second

// releaser runs debug.FreeOSMemory once things have been quiet for releaseAfter.
type releaser struct {
	on    bool // only the window: not tests or the development harness
	mu    sync.Mutex
	timer *time.Timer
}

func (r *releaser) soon() {
	if !r.on {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.timer != nil {
		r.timer.Reset(releaseAfter)
		return
	}
	r.timer = time.AfterFunc(releaseAfter, debug.FreeOSMemory)
}

// Leave is called by the page when the player leaves a page (screens.js): what only that
// page used is forgotten and the memory goes back to the OS.
func (s *API) Leave(page string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch page {
	case "library":
		s.lib, s.libAt = nil, ""
	case "needs", "log":
		s.a.DropLogCaches()
		s.logLinks = nil
	case "home":
		s.a.DropPlanProblems()
	}
	s.free.soon()
}
