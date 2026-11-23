//go:build windows || cgo

package gui

import "testing"

func TestSideStep(t *testing.T) {
	const press, double, triple, release = 4, 5, 6, 7
	for _, tc := range []struct {
		eventType, button uint32
		step              int
		swallow           bool
	}{
		{press, 8, -1, true},
		{press, 9, 1, true},
		{double, 8, 0, true}, // a quick second press already sent its own GDK_BUTTON_PRESS
		{triple, 9, 0, true},
		{release, 8, 0, true},
		{press, 1, 0, false},
		{press, 3, 0, false}, // the right button stays the page's (its own menu, 16 §8.5)
		{double, 1, 0, false},
	} {
		step, swallow := sideStep(tc.eventType, tc.button)
		if step != tc.step || swallow != tc.swallow {
			t.Errorf("sideStep(%d, %d) = %d, %v; want %d, %v", tc.eventType, tc.button, step, swallow, tc.step, tc.swallow)
		}
	}
}
