//go:build !linux && (windows || cgo)

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

// wireSideButtons: WebView2 hands the side buttons to the page as buttons 3 and 4, which
// navigation.js handles itself (16 §3.1 rule 11). Only the Linux window needs a hook.
func wireSideButtons(*application.WebviewWindow, *API) {}
