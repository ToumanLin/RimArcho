//go:build !windows && !linux

// Package upgradewin shows a small native window while the database upgrades,
// before the GUI window opens (04 §4, 16 §8.1).
package upgradewin

// Window is unused outside Windows and Linux. Its methods are safe on a nil receiver.
type Window struct{}

// Open returns nil here.
func Open(title, text string) *Window { return nil }

// Update does nothing here.
func (w *Window) Update(text string, frac float64) {}

// Close does nothing here.
func (w *Window) Close() {}
