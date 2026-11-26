//go:build windows

package gui

import (
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"golang.org/x/sys/windows"
)

// WebView2's popup lets clicks through (16 §8.2 "Other virtual desktops").
//
// Under composition hosting the WebView2 browser process opens a top-level window of its own
// (class Chrome_WidgetWin_1; layered, no redirection bitmap, tool window; no owner) with the
// window's size and position, just below it. Windows cloaks a window on the virtual desktops
// it is not on, but a tool window with no owner belongs to no desktop and is never cloaked:
// on every other desktop that invisible popup lay on the wallpaper and took its clicks
// (issue #29). RimArcho adds WS_EX_TRANSPARENT to it, so hit testing passes through it on
// every desktop. On RimArcho's own desktop nothing changes: the window above it takes the
// input, as before. It is done when the runtime is ready and again on each activation, in
// case the browser process was restarted with a new popup.

var (
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procGetWindow                = user32.NewProc("GetWindow")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW        = user32.NewProc("SetWindowLongPtrW")
)

const (
	gwOwner    = 4
	gwlExStyle = -20

	wsExTransparent         = 0x00000020
	wsExToolWindow          = 0x00000080
	wsExLayered             = 0x00080000
	wsExNoRedirectionBitmap = 0x00200000

	webviewPopupClass = "Chrome_WidgetWin_1"
	// webviewPopupStyle is the popup's signature. Chromium's real popups (a <select> list,
	// a tooltip) are smaller than the window, which the rectangle test also rules out.
	webviewPopupStyle = wsExLayered | wsExNoRedirectionBitmap | wsExToolWindow
)

type rect struct{ left, top, right, bottom int32 }

// popupWindow is an unowned top-level window of webviewPopupClass.
type popupWindow struct {
	hwnd uintptr
	pid  uint32
	ex   uintptr
	r    rect
}

// webviewPopup says whether w is WebView2's popup for the window at main, in one of
// RimArcho's own child processes (the WebView2 browser).
func webviewPopup(w popupWindow, main rect, children map[uint32]bool) bool {
	return children[w.pid] && w.ex&webviewPopupStyle == webviewPopupStyle && w.r == main
}

// followDesktops wires passThroughWebviewPopup to the window's events.
func followDesktops(win *application.WebviewWindow) {
	fix := func(*application.WindowEvent) {
		application.InvokeAsync(func() { passThroughWebviewPopup(uintptr(win.NativeWindow())) })
	}
	win.OnWindowEvent(events.Common.WindowRuntimeReady, fix)
	win.OnWindowEvent(events.Windows.WindowActive, fix)
}

// The EnumWindows callback is made once: Windows has room for a limited number of them.
var (
	popupsFound []popupWindow
	popupChild  = map[uint32]bool{}
	popupClass  = make([]uint16, 64)
	popupEnum   = syscall.NewCallback(func(h, _ uintptr) uintptr {
		if owner, _, _ := procGetWindow.Call(h, gwOwner); owner != 0 {
			return 1
		}
		n, _, _ := procGetClassNameW.Call(h, uintptr(unsafe.Pointer(&popupClass[0])), uintptr(len(popupClass)))
		if windows.UTF16ToString(popupClass[:n]) != webviewPopupClass {
			return 1
		}
		w := popupWindow{hwnd: h, ex: exStyle(h)}
		procGetWindowThreadProcessID.Call(h, uintptr(unsafe.Pointer(&w.pid)))
		procGetWindowRect.Call(h, uintptr(unsafe.Pointer(&w.r)))
		popupsFound = append(popupsFound, w)
		return 1
	})
)

func exStyle(h uintptr) uintptr {
	index := gwlExStyle // a variable: a negative constant does not convert to uintptr
	ex, _, _ := procGetWindowLongPtrW.Call(h, uintptr(index))
	return ex
}

// passThroughWebviewPopup makes WebView2's popup for main transparent to the mouse and
// returns how many windows it changed. It runs on the UI thread, which keeps popupsFound
// and popupChild to one caller at a time.
func passThroughWebviewPopup(main uintptr) int {
	if main == 0 {
		return 0
	}
	var mainRect rect
	if ok, _, _ := procGetWindowRect.Call(main, uintptr(unsafe.Pointer(&mainRect))); ok == 0 {
		return 0
	}
	popupsFound = popupsFound[:0]
	procEnumWindows.Call(popupEnum, 0)
	// Browsers and Electron apps have such windows too. Whether a process is RimArcho's
	// child is remembered, so a process snapshot is taken only when a new one shows up.
	for _, w := range popupsFound {
		if _, known := popupChild[w.pid]; !known {
			children := childProcesses(uint32(windows.GetCurrentProcessId()))
			for _, w := range popupsFound {
				popupChild[w.pid] = children[w.pid]
			}
			break
		}
	}
	changed := 0
	for _, w := range popupsFound {
		if webviewPopup(w, mainRect, popupChild) && w.ex&wsExTransparent == 0 {
			index := gwlExStyle
			procSetWindowLongPtrW.Call(w.hwnd, uintptr(index), w.ex|wsExTransparent)
			changed++
		}
	}
	return changed
}

// childProcesses are the IDs of parent's direct child processes.
func childProcesses(parent uint32) map[uint32]bool {
	out := map[uint32]bool{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ParentProcessID == parent {
			out[e.ProcessID] = true
		}
	}
	return out
}
