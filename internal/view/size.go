package view

import "github.com/ToumanLin/RimArcho/internal/render"

// SizeText is a size for the interfaces: GB from 1 GB up, MB below. The window, the TUI
// and the commands (render.Size) all show sizes this way.
func SizeText(n int64) string { return render.Size(n) }
