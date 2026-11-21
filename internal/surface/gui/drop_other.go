//go:build !(windows && (amd64 || arm64))

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

// wireNativeDrop: elsewhere the window's own file drop is not used yet (16 §2.18).
func wireNativeDrop(*application.WebviewWindow, *API) {}
