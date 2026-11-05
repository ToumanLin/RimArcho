package gui

import (
	"runtime/debug"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/perftrace"
)

// The window process's memory (07 §8.3). Go does not collect on its own while the window
// sits idle, so what a page built stays resident after the page is gone; the window returns
// it a moment after heavy work. Change-keyed inventory/log caches survive navigation
// so revisiting a page does not repeat the same database queries or log analysis.

// MemoryLimit is the window process's Go soft memory cap: only a ceiling against runaway
// growth. Idle memory comes from releasing after heavy work, not from the cap; at 32 MiB a
// refresh's live heap sat near the cap and one list edit ran hundreds of collections.
// Retained WAL readers for a 5,000-mod library need about 100 MiB live; 128 MiB
// leaves too little space for concurrent page construction and causes the same GC
// thrashing. The cap leaves allocation headroom; idle release still returns it.
const MemoryLimit = 256 << 20

// tuneProcess applies the window's memory settings: the soft cap, and a 256 KiB SQLite page
// cache on the main connection, as in the TUI. ReadSession applies the same cache size to
// each page connection (the default 2 MB stays resident in modernc's allocator).
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

// Leave cancels abandoned texture reads and drops transient page state. Dedicated
// page readers keep their change-keyed caches for the next visit.
func (s *API) Leave(page string) {
	defer perftrace.Span("Leave")()
	if page == "plans" || page == "needs" || page == "home" {
		s.cancelLoadCost(false)
	}
	if page == "search" {
		s.stopFileSearch()
	}
	if page == "saves" { // view.PageSaves
		s.leaveSaves()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch page {
	case "textures":
		if s.texCancel != nil {
			s.texCancel()
			s.texCancel = nil
		}
	case "needs", "log":
		s.a.DropLogCaches()
		s.logLinks = nil
	case "home":
		s.a.DropPlanProblems()
	}
	s.free.soon()
}
