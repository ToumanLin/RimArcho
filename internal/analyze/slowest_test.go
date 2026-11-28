package analyze

import (
	"testing"
	"time"
)

// slowest keeps the n longest, slowest first, whatever order they come in.
func TestSlowestKeepsTopN(t *testing.T) {
	s := slowest[int]{n: 3}
	for i, d := range []int{5, 1, 9, 3, 9, 7, 2} {
		s.add(i, time.Duration(d))
	}
	want := []time.Duration{9, 9, 7}
	if len(s.durs) != 3 || s.durs[0] != want[0] || s.durs[1] != want[1] || s.durs[2] != want[2] || s.items[0] != 2 || s.items[2] != 5 {
		t.Fatalf("%v %v", s.items, s.durs)
	}
}
