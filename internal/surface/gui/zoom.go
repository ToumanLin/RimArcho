package gui

import (
	"strconv"
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
