package gui

import (
	"sync"
	"time"
)

// WebView2's memory target follows where the window is (07 §8.3, 06 §2.16.1 M08): low while
// minimised, and low once the window has stayed in the background for backgroundLow, as
// while the player is in the game. Back to normal as soon as it is active again. Low makes
// WebView2 trim its caches, collect and drop painted tiles; on a 6,574-copy synthetic home
// the page process went from 78 to 44 MB and stayed lower after restore.

// backgroundLow is how long an inactive window waits before asking for low memory, so a
// quick switch to another window and back changes nothing.
var backgroundLow = time.Minute

const (
	memoryTargetNormal = 0 // COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_NORMAL
	memoryTargetLow    = 1
)

// memoryTarget decides the level from window events; set applies it (on the UI thread).
type memoryTarget struct {
	mu        sync.Mutex
	set       func(level uintptr)
	timer     *time.Timer
	minimised bool
	inactive  bool
	level     uintptr
}

func (m *memoryTarget) apply(level uintptr) {
	if m.level != level {
		m.level = level
		m.set(level)
	}
}

func (m *memoryTarget) stopTimer() {
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
}

// Inactive: another window took the focus.
func (m *memoryTarget) Inactive() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inactive = true
	if m.timer == nil && !m.minimised {
		m.timer = time.AfterFunc(backgroundLow, m.background)
	}
}

func (m *memoryTarget) background() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer == nil { // overtaken by Active or Minimised
		return
	}
	m.timer = nil
	if m.inactive {
		m.apply(memoryTargetLow)
	}
}

// Active: the window has the focus again.
func (m *memoryTarget) Active() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inactive = false
	m.stopTimer()
	if !m.minimised {
		m.apply(memoryTargetNormal)
	}
}

func (m *memoryTarget) Minimised() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.minimised = true
	m.stopTimer()
	m.apply(memoryTargetLow)
}

// Restored counts as active: Windows activates a window while it is still minimised, and
// Wails reports no WindowActive for that activation. Wrongly assuming active only keeps
// normal memory; wrongly assuming inactive would trim a window the player is using.
func (m *memoryTarget) Restored() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.minimised, m.inactive = false, false
	m.stopTimer()
	m.apply(memoryTargetNormal)
}

// Stop ends a pending background timer when the window closes.
func (m *memoryTarget) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopTimer()
}
