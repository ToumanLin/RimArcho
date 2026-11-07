package gui

import (
	"sync"
	"time"
)

// WebView2's memory target follows where the window is and whether it is used (17 §7.3,
// 16 §2.16.1 M08): low while minimised, once the window has stayed in the background for
// backgroundLow (as while the player is in the game), and once the active window has had
// no input for idleLow. Back to normal at once on input, activation or restore. Low makes
// WebView2 trim its caches, collect and drop painted tiles; a visible window keeps drawing
// and redraws as it is used. On a 6,574-copy synthetic home the page process went from 78
// to 44 MB when minimised and from about 100 to 15 MB when left in the background.

// backgroundLow is how long an inactive window waits before asking for low memory, so a
// quick switch to another window and back changes nothing.
var backgroundLow = time.Minute

// idleLow is how long the active window waits for input before asking for low memory. The
// page reports input at most every inputEvery, so low comes between idleLow-inputEvery and
// idleLow after the last input.
var idleLow = 2 * time.Minute

const (
	memoryTargetNormal = 0 // COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_NORMAL
	memoryTargetLow    = 1
)

// memoryTarget decides the level from window events and page input; set applies it (on
// the UI thread).
type memoryTarget struct {
	mu        sync.Mutex
	set       func(level uintptr)
	timer     *time.Timer // the background or idle wait, whichever applies
	minimised bool
	inactive  bool
	waited    bool // the wait for the current state ran out
	level     uintptr
}

// update starts the wait that fits the window's state and applies the level.
func (m *memoryTarget) update() {
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	if !m.minimised && !m.waited {
		wait := idleLow
		if m.inactive {
			wait = backgroundLow
		}
		var t *time.Timer
		t = time.AfterFunc(wait, func() { m.ranOut(t) })
		m.timer = t
	}
	level := uintptr(memoryTargetNormal)
	if m.minimised || m.waited {
		level = memoryTargetLow
	}
	if m.level != level {
		m.level = level
		m.set(level)
	}
}

func (m *memoryTarget) ranOut(t *time.Timer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer != t { // overtaken by another event
		return
	}
	m.timer = nil
	m.waited = true
	m.update()
}

// Inactive: another window took the focus. An idle window already low stays low.
func (m *memoryTarget) Inactive() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inactive {
		return
	}
	m.inactive = true
	if !m.waited {
		m.update() // the background wait replaces the idle wait
	}
}

// Active: the window has the focus again.
func (m *memoryTarget) Active() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inactive, m.waited = false, false
	m.update()
}

// Input: the player used the page (pointer, wheel or key), which also happens in an
// inactive window, as wheel scrolling under the pointer.
func (m *memoryTarget) Input() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.waited = false
	m.update()
}

func (m *memoryTarget) Minimised() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.minimised = true
	m.update()
}

// Restored counts as active: Windows activates a window while it is still minimised, and
// Wails reports no WindowActive for that activation. Wrongly assuming active only keeps
// normal memory; wrongly assuming inactive would trim a window the player is using.
func (m *memoryTarget) Restored() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.minimised, m.inactive, m.waited = false, false, false
	m.update()
}

// Stop ends a pending wait when the window closes. Closing can still be refused (an
// uninstall in progress); the next event starts a wait again.
func (m *memoryTarget) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
}

// PageInput: the page reports the player's input, at most every inputEvery (js/activity.js).
func (s *API) PageInput() {
	if s.memTarget != nil {
		s.memTarget.Input()
	}
}
