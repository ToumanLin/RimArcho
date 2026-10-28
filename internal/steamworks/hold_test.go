package steamworks

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) clock() watchClock {
	return watchClock{now: func() time.Time { return c.t }, sleep: func(d time.Duration) { c.t = c.t.Add(d) }}
}

// A subscription refresh that lands after the worker subscribed turns item 2 back; the
// worker asks again for it only, once, and keeps the process up for the answer. Item 3 was
// refused, so it is left alone.
func TestHoldChangesAsksAgainForTurnedBack(t *testing.T) {
	fc := &fakeClock{t: time.Unix(0, 0)}
	start := fc.t
	state := map[uint64]uint32{1: ItemSubscribed, 2: ItemSubscribed, 3: 0}
	again := map[uint64]int{}
	var lastAgain time.Time
	api := holdAPI{
		state: func(id uint64) uint32 { return state[id] },
		again: func(id uint64) {
			again[id]++
			lastAgain = fc.t
			state[id] = ItemSubscribed
		},
		callbacks: func() {
			// the stale list arrives 1.2 s in and drops item 2
			if fc.t.Sub(start) >= 1200*time.Millisecond && again[2] == 0 {
				state[2] = 0
			}
		},
	}
	asked := holdChanges([]uint64{1, 2, 3}, []bool{true, true, false}, true, api, fc.clock(), holdTime)
	if asked[0] || !asked[1] || asked[2] || again[2] != 1 || again[1] != 0 || again[3] != 0 {
		t.Fatalf("asked %v again %v", asked, again)
	}
	if fc.t.Sub(lastAgain) < time.Second {
		t.Fatalf("returned %v after asking again", fc.t.Sub(lastAgain))
	}
}

// Unsubscribing: an item that shows subscribed again is unsubscribed once more.
func TestHoldChangesUnsubscribe(t *testing.T) {
	fc := &fakeClock{t: time.Unix(0, 0)}
	state := map[uint64]uint32{7: ItemSubscribed | ItemInstalled}
	n := 0
	api := holdAPI{state: func(id uint64) uint32 { return state[id] }, again: func(id uint64) { n++; state[id] = 0 }, callbacks: func() {}}
	if asked := holdChanges([]uint64{7}, []bool{true}, false, api, fc.clock(), holdTime); !asked[0] || n != 1 {
		t.Fatalf("asked %v n %d", asked, n)
	}
}

// settle runs the callbacks for its whole time.
func TestSettleRunsCallbacks(t *testing.T) {
	fc := &fakeClock{t: time.Unix(0, 0)}
	n := 0
	settle(func() { n++ }, fc.clock(), settleTime)
	if n < 15 || fc.t.Sub(time.Unix(0, 0)) < settleTime {
		t.Fatalf("callbacks %d over %v", n, fc.t.Sub(time.Unix(0, 0)))
	}
}
