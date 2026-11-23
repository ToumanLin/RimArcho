//go:build !(windows && (amd64 || arm64)) && !linux && (windows || cgo)

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

// wireNativeDrop: on macOS and 32-bit Windows the window's own file drop is not used yet
// (16 §2.18).
func wireNativeDrop(*application.WebviewWindow, *API) {}
