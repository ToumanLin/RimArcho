//go:build !windows

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

func newWindowTheme(preference string) *windowTheme {
	bg, _, _ := framePalette(preference == "dark")
	return &windowTheme{background: bg}
}

func (f *windowTheme) bind(w *application.WebviewWindow) func(bool) {
	return func(dark bool) {
		bg, _, _ := framePalette(dark)
		w.SetBackgroundColour(bg)
	}
}
