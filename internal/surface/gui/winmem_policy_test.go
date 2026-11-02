package gui

import (
	"slices"
	"sync"
	"testing"
	"time"
)

func TestMemoryTargetFollowsWindow(t *testing.T) {
	oldBg, oldIdle := backgroundLow, idleLow
	backgroundLow, idleLow = 30*time.Millisecond, time.Hour
	defer func() { backgroundLow, idleLow = oldBg, oldIdle }()
	var mu sync.Mutex
	var got []uintptr
	m := &memoryTarget{set: func(l uintptr) { mu.Lock(); got = append(got, l); mu.Unlock() }}
	levels := func() []uintptr { mu.Lock(); defer mu.Unlock(); l := got; got = nil; return l }
	wait := func() { time.Sleep(4 * backgroundLow) }
	expect := func(what string, want ...uintptr) {
		t.Helper()
		if l := levels(); !slices.Equal(l, want) {
			t.Fatalf("%s: %v, want %v", what, l, want)
		}
	}

	m.Input()
	// A quick switch away and back changes nothing.
	m.Inactive()
	m.Active()
	wait()
	expect("quick switch")
	// In the background long enough: low; active again: normal.
	m.Inactive()
	wait()
	m.Active()
	expect("background then active", memoryTargetLow, memoryTargetNormal)
	// Minimised: low at once, once; restored: normal, even before an Active event
	// (Wails reports none when Windows activates a still-minimised window).
	m.Inactive()
	m.Minimised()
	wait()
	m.Restored()
	wait()
	expect("minimised then restored", memoryTargetLow, memoryTargetNormal)
	// Closing stops the wait.
	m.Inactive()
	m.Stop()
	wait()
	expect("after close")
}

func TestMemoryTargetIdleInFront(t *testing.T) {
	oldBg, oldIdle := backgroundLow, idleLow
	backgroundLow, idleLow = time.Hour, 60*time.Millisecond
	defer func() { backgroundLow, idleLow = oldBg, oldIdle }()
	var mu sync.Mutex
	var got []uintptr
	m := &memoryTarget{set: func(l uintptr) { mu.Lock(); got = append(got, l); mu.Unlock() }}
	levels := func() []uintptr { mu.Lock(); defer mu.Unlock(); l := got; got = nil; return l }
	wait := func() { time.Sleep(4 * idleLow) }
	expect := func(what string, want ...uintptr) {
		t.Helper()
		if l := levels(); !slices.Equal(l, want) {
			t.Fatalf("%s: %v, want %v", what, l, want)
		}
	}

	// Input keeps the active window normal; without it the window goes low.
	m.Input()
	for range 6 {
		time.Sleep(idleLow / 6)
		m.Input()
	}
	expect("kept in use")
	wait()
	expect("idle", memoryTargetLow)
	// The next input brings it back at once, and the wait starts again.
	m.Input()
	expect("input after idle", memoryTargetNormal)
	wait()
	expect("idle again", memoryTargetLow)
	// An idle window that loses the focus stays low; activation brings it back.
	m.Inactive()
	wait()
	expect("idle then inactive")
	m.Active()
	expect("active after idle", memoryTargetNormal)
	// The idle wait does not apply to a background window: it uses backgroundLow (an hour
	// here), and wheel input under the pointer still counts.
	m.Inactive()
	wait()
	expect("inactive before backgroundLow")
	m.Input()
	wait()
	expect("input in an inactive window")
	m.Stop()
}
