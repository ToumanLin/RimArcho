package main

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/corpus"
)

func TestWeekStart(t *testing.T) {
	for _, w := range []string{"2026-W01", "2026-W47", "2025-W53", "2026-W53", "2027-W01", "2020-W53"} {
		s, err := weekStart(w)
		if w == "2025-W53" {
			if err == nil {
				t.Fatalf("2025 has no week 53, got %v", s)
			}
			continue
		}
		if err != nil || corpus.Week(s) != w || s.Weekday() != 1 {
			t.Fatalf("%s: %v %v", w, s, err)
		}
	}
}
