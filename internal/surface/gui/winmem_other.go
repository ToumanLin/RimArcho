//go:build !windows

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

// lowMemoryWhenAway is WebView2's (Windows only).
func lowMemoryWhenAway(*application.WebviewWindow) *memoryTarget { return nil }
