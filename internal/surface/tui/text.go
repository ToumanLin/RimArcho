package tui

import "github.com/ToumanLin/RimArcho/internal/view"

// The interface text lives in internal/view (06 §1); the TUI adds its own help text.

// T translates an interface string.
func T(s string) string { return view.T(s) }

// Tf translates a format string, then formats.
func Tf(f string, a ...any) string { return view.Tf(f, a...) }

// SetLang selects the interface language.
func SetLang(v string) { view.SetLang(v) }
