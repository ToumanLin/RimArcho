//go:build !linux && (windows || cgo)

package gui

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// The window needs nothing more on Windows and macOS (platform_linux.go has the Linux part).

func platformApp(*app.App, *application.Options)                       {}
func platformWindow(*app.App, *application.WebviewWindowOptions)       {}
func platformStart(*application.App, *application.WebviewWindow) error { return nil }
