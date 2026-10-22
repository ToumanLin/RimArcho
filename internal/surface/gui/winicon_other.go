//go:build !windows

package gui

import "github.com/ToumanLin/RimArcho/internal/icon"

// windowIcon is the application icon for the window (06 §8.6).
func windowIcon() []byte { return icon.PNG(256) }
