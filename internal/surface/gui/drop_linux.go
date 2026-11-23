//go:build cgo

package gui

import (
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// wireNativeDrop: Wails' GTK drop target (EnableFileDrop, set by platformWindow) hands the
// dropped files to the page's runtime, which reports a drop on an element marked
// data-file-drop-target; exchange.js marks the whole page. They go on as "files-dropped",
// as on Windows (16 §2.18 "Dropping files"). WebKitGTK hands over files only, not links.
func wireNativeDrop(win *application.WebviewWindow, s *API) {
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		paths := e.Context().DroppedFiles()
		d := dropData{Paths: paths, Dirs: make([]bool, len(paths))}
		for i, p := range paths {
			fi, err := os.Stat(p)
			d.Dirs[i] = err == nil && fi.IsDir()
		}
		s.send("files-dropped", d)
	})
}
