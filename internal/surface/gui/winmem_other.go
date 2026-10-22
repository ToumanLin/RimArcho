//go:build !windows

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

// lowMemoryWhenMinimised is WebView2's (Windows only).
func lowMemoryWhenMinimised(*application.WebviewWindow) {}
