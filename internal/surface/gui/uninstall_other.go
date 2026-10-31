//go:build !windows

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

func (s *API) uninstallSessionEnd(_ uintptr, _ uint32, _, _ uintptr) (uintptr, bool) { return 0, false }
func uninstallShutdownBlock(*application.WebviewWindow) func(bool)                   { return nil }
