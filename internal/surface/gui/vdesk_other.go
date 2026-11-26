//go:build !windows && cgo

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

// followDesktops is for WebView2's popup (Windows only, vdesk_windows.go).
func followDesktops(*application.WebviewWindow) {}
