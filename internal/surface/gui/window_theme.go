package gui

import "github.com/wailsapp/wails/v3/pkg/application"

// SetWindowTheme receives the resolved scheme, including changes to the OS theme
// while the appearance setting is auto. It does not change the saved preference.
func (s *API) SetWindowTheme(dark bool) {
	if s.windowTheme != nil {
		s.windowTheme(dark)
	}
}

type windowTheme struct {
	options    application.WindowsWindow
	background application.RGBA
	dark       bool
}

// Keep these colours aligned with --bg, --fg and --muted in style.css.
func framePalette(dark bool) (background, text, inactive application.RGBA) {
	if dark {
		return application.NewRGB(0x1a, 0x1a, 0x1e), application.NewRGB(0xed, 0xed, 0xf1), application.NewRGB(0x8b, 0x8b, 0x94)
	}
	return application.NewRGB(0xf4, 0xf4, 0xf6), application.NewRGB(0x1c, 0x1c, 0x21), application.NewRGB(0x85, 0x85, 0x8d)
}
