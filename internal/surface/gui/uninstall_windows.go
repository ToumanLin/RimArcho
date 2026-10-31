package gui

import (
	"syscall"
	"unsafe"

	"github.com/ToumanLin/RimArcho/internal/view"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

const wmQueryEndSession = 0x0011

// Wails forwards WM_QUERYENDSESSION to DefWindowProc by default. Refuse this
// session-end attempt while uninstalling; Windows still permits a forced shutdown.
func (s *API) uninstallSessionEnd(_ uintptr, msg uint32, _, _ uintptr) (uintptr, bool) {
	if msg == wmQueryEndSession && !s.allowSessionEnd() {
		return 0, true
	}
	return 0, false
}

func (s *API) allowSessionEnd() bool {
	// A cancelled shutdown must not leave normal uninstall startup locked out.
	if s.UninstallStatus().Running {
		s.send("uninstall-close-blocked", nil)
		return false
	}
	return true
}

func uninstallShutdownBlock(win *application.WebviewWindow) func(bool) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	create := user32.NewProc("ShutdownBlockReasonCreate")
	destroy := user32.NewProc("ShutdownBlockReasonDestroy")
	return func(active bool) {
		application.InvokeSync(func() {
			hwnd := uintptr(win.NativeWindow())
			if hwnd == 0 {
				return
			}
			if active {
				reason, _ := syscall.UTF16PtrFromString(view.UninstallCloseReason())
				create.Call(hwnd, uintptr(unsafe.Pointer(reason)))
			} else {
				destroy.Call(hwnd)
			}
		})
	}
}
