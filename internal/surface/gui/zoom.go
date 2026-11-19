package gui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// windowZoom applies settings › interface size (ui.zoom, 16 §2.14) to the window: WebView2's
// zoom factor, which scales text and controls together, as the browser's zoom does, so the
// page's layouts and virtual rows keep their CSS pixels.
type windowZoom struct {
	mu   sync.Mutex
	set  func(float64) // the window's SetZoom; nil in the harness and in tests
	last float64
}

// ZoomFactor is ui.zoom as a factor: "125" is 1.25.
func ZoomFactor(a *app.App) float64 {
	pct, err := strconv.Atoi(cmds.Setting(a, "ui.zoom"))
	if err != nil || pct < 100 {
		return 1
	}
	return float64(pct) / 100
}

// apply sets the window's zoom when the setting changed since the last call.
func (z *windowZoom) apply(f float64) {
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.set == nil || f == z.last {
		return
	}
	z.last = f
	z.set(f)
}

// StepZoom is ctrl+wheel and ctrl+'+' / '-' / '0' on any page (16 §2.14, §3): one step
// along settings › interface size's choices (step > 0 larger, < 0 smaller, 0 back to the
// default), stored as ui.zoom like the setting, so the window reopens at it. The window
// takes the new factor at once. It returns the size now set, in percent.
func (s *API) StepZoom(step int) (int, error) {
	s.mu.Lock()
	cur, _ := strconv.Atoi(cmds.Setting(s.a, "ui.zoom"))
	next := nextZoom(cur, zoomChoices(), step)
	if next != cur {
		if out, code := cmds.Execute(s.a, []string{"config", "set", "ui.zoom", strconv.Itoa(next)}); code != cmds.ExitOK {
			s.mu.Unlock()
			return cur, fmt.Errorf("%s", strings.TrimSpace(out))
		}
	}
	s.mu.Unlock()
	// outside the lock: SetZoom waits for the window's thread
	s.zoom.apply(float64(next) / 100)
	return next, nil
}

// zoomChoices are ui.zoom's choices as percentages, smallest first.
func zoomChoices() []int {
	var out []int
	for _, c := range cmds.SettingChoices("ui.zoom") {
		if n, err := strconv.Atoi(c); err == nil {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// nextZoom snaps to the choices: from a size between two choices a step lands on the next
// one in its direction; past either end it stays there; step 0 is the smallest (100%).
func nextZoom(cur int, choices []int, step int) int {
	if len(choices) == 0 {
		return 100
	}
	switch {
	case step > 0:
		for _, c := range choices {
			if c > cur {
				return c
			}
		}
		return choices[len(choices)-1]
	case step < 0:
		for i := len(choices) - 1; i >= 0; i-- {
			if choices[i] < cur {
				return choices[i]
			}
		}
		return choices[0]
	}
	return choices[0]
}
