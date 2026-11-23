//go:build !windows && cgo

package gui

import "github.com/ToumanLin/RimArcho/internal/icon"

// windowIcon is the application icon for the window (16 §8.6): 64 px, drawn in about 35 ms
// with 9 MB of buffers (256 px took 0.2 s and 65 MB at every start).
func windowIcon() []byte { return icon.PNG(64) }
