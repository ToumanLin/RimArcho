//go:build !windows

// Package upgradewin shows a small native window while the database upgrades,
// before the GUI window opens (02 §7, 06 §8.1).
package upgradewin

// Window is unused outside Windows. Its methods are safe on a nil receiver.
type Window struct{}

// Open returns nil outside Windows.
func Open(title, text string) *Window { return nil }

// Update does nothing outside Windows.
func (w *Window) Update(text string, frac float64) {}

// Close does nothing outside Windows.
func (w *Window) Close() {}
