package gui

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

func colourRef(c application.RGBA) *uint32 {
	v := uint32(c.Red) | uint32(c.Green)<<8 | uint32(c.Blue)<<16
	return &v
}

func newWindowTheme(preference string) *windowTheme {
	f := &windowTheme{dark: preference == "dark" || (preference != "light" && w32.IsCurrentlyDarkMode())}
	f.options.Theme = application.SystemDefault
	if preference == "dark" {
		f.options.Theme = application.Dark
	} else if preference == "light" {
		f.options.Theme = application.Light
	}
	f.palette(f.dark)
	return f
}

func (f *windowTheme) palette(dark bool) {
	f.dark = dark
	bg, fg, muted := framePalette(dark)
	f.background = bg
	active := &application.WindowTheme{TitleBarColour: colourRef(bg), TitleTextColour: colourRef(fg)}
	inactive := &application.WindowTheme{TitleBarColour: colourRef(bg), TitleTextColour: colourRef(muted)}
	// Wails reapplies these on activation and system theme changes. Both OS
	// branches use the page's current scheme, including an explicit override.
	f.options.CustomTheme = application.ThemeSettings{
		DarkModeActive: active, LightModeActive: active,
		DarkModeInactive: inactive, LightModeInactive: inactive,
	}
}

func (f *windowTheme) bind(w *application.WebviewWindow) func(bool) {
	// The options were copied when the window was created; retain and update
	// the shared theme values rather than replacing their pointers.
	active, inactive := f.options.CustomTheme.DarkModeActive, f.options.CustomTheme.DarkModeInactive
	apply := func() {
		if w32.IsCurrentlyHighContrastMode() {
			return
		}
		hwnd := uintptr(w.NativeWindow())
		if hwnd == 0 {
			return
		}
		w32.SetTheme(hwnd, f.dark)
		bg, fg, muted := framePalette(f.dark)
		if w32.GetForegroundWindow() != hwnd {
			fg = muted
		}
		w32.SetTitleBarColour(hwnd, *colourRef(bg))
		w32.SetTitleTextColour(hwnd, *colourRef(fg))
	}
	for _, event := range []events.WindowEventType{events.Common.WindowRuntimeReady, events.Windows.WindowActive, events.Windows.WindowInactive} {
		w.OnWindowEvent(event, func(*application.WindowEvent) { application.InvokeAsync(apply) })
	}
	return func(dark bool) {
		application.InvokeAsync(func() {
			f.dark = dark
			bg, fg, muted := framePalette(dark)
			*active.TitleBarColour, *inactive.TitleBarColour = *colourRef(bg), *colourRef(bg)
			*active.TitleTextColour, *inactive.TitleTextColour = *colourRef(fg), *colourRef(muted)
			w.SetBackgroundColour(bg)
			apply()
		})
	}
}
