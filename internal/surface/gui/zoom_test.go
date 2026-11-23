//go:build windows || cgo

package gui

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// 16 §2.14: ctrl+wheel and ctrl+'+' / '-' / '0' step along interface size's choices.
func TestNextZoomSnapsToChoices(t *testing.T) {
	t.Parallel()
	steps := []int{100, 110, 125, 150}
	for _, c := range []struct{ cur, step, want int }{
		{100, 1, 110}, {110, 1, 125}, {150, 1, 150}, {125, -1, 110}, {100, -1, 100},
		{150, 0, 100}, {115, 1, 125}, {115, -1, 110}, {0, 1, 100}, {300, -1, 150},
	} {
		if got := nextZoom(c.cur, steps, c.step); got != c.want {
			t.Errorf("nextZoom(%d, %d) = %d, want %d", c.cur, c.step, got, c.want)
		}
	}
}

// The step is stored as ui.zoom, as the setting is, and reaches the window at once.
func TestStepZoomStoresAndApplies(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	var applied []float64
	api.zoom.last, api.zoom.set = 1, func(f float64) { applied = append(applied, f) }
	for _, c := range []struct{ step, want int }{{1, 110}, {1, 125}, {-1, 110}, {0, 100}, {-1, 100}} {
		got, err := api.StepZoom(c.step)
		if err != nil || got != c.want {
			t.Fatalf("StepZoom(%d) = %d %v, want %d", c.step, got, err, c.want)
		}
	}
	if v := cmds.Setting(e.App, "ui.zoom"); v != "100" {
		t.Fatalf("ui.zoom = %s", v)
	}
	if want := []float64{1.1, 1.25, 1.1, 1}; len(applied) != len(want) || applied[0] != want[0] || applied[1] != want[1] || applied[2] != want[2] || applied[3] != want[3] {
		t.Fatalf("window zoom %v, want %v (an unchanged step sets nothing)", applied, want)
	}
	if _, err := api.StepZoom(1); err != nil || ZoomFactor(e.App) != 1.1 {
		t.Fatalf("the next start opens at %v", ZoomFactor(e.App))
	}
}
