package gui

import (
	"slices"
	"sync"
	"testing"
	"time"
)

func TestMemoryTargetFollowsWindow(t *testing.T) {
	old := backgroundLow
	backgroundLow = 30 * time.Millisecond
	defer func() { backgroundLow = old }()
	var mu sync.Mutex
	var got []uintptr
	m := &memoryTarget{set: func(l uintptr) { mu.Lock(); got = append(got, l); mu.Unlock() }}
	levels := func() []uintptr { mu.Lock(); defer mu.Unlock(); return slices.Clone(got) }
	wait := func() { time.Sleep(4 * backgroundLow) }

	// A quick switch away and back changes nothing.
	m.Inactive()
	m.Active()
	wait()
	if l := levels(); len(l) != 0 {
		t.Fatalf("quick switch set %v", l)
	}
	// In the background long enough: low; active again: normal.
	m.Inactive()
	wait()
	m.Active()
	if l := levels(); !slices.Equal(l, []uintptr{memoryTargetLow, memoryTargetNormal}) {
		t.Fatalf("background then active: %v", l)
	}
	// Minimised: low at once, once; restored: normal, even before an Active event
	// (Wails reports none when Windows activates a still-minimised window).
	mu.Lock()
	got = nil
	mu.Unlock()
	m.Inactive()
	m.Minimised()
	wait()
	m.Restored()
	wait()
	if l := levels(); !slices.Equal(l, []uintptr{memoryTargetLow, memoryTargetNormal}) {
		t.Fatalf("minimised then restored: %v", l)
	}
	// Losing the focus afterwards starts the wait again; closing stops it.
	m.Inactive()
	m.Stop()
	wait()
	if l := levels(); !slices.Equal(l, []uintptr{memoryTargetLow, memoryTargetNormal}) {
		t.Fatalf("after close: %v", l)
	}
}
