package gui

import (
	"reflect"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// WebView2's memory target follows the window (winmem_policy.go). This file reaches it.
//
// Wails beta.28 does not expose it. Its internal binding of ICoreWebView2_19 lists only the
// interface's own two methods after IUnknown, so it would call get_Settings; the call here
// goes to the right slot of the real vtable. The ICoreWebView2 pointer is read from Wails'
// private fields by name; with another layout (another Wails version) the hook does nothing.

// iidWebView2_19 is IID_ICoreWebView2_19.
var iidWebView2_19 = syscall.GUID{Data1: 0x6921f954, Data2: 0x79b0, Data3: 0x437f, Data4: [8]byte{0xa9, 0x97, 0xc8, 0x58, 0x11, 0x89, 0x7c, 0x68}}

const (
	slotQueryInterface = 0
	slotRelease        = 2
	// put_MemoryUsageTargetLevel: IUnknown (3), then ICoreWebView2 (58) and _2 … _18 (7, 5,
	// 4, 2, 1, 1, 7, 9, 2, 3, 3, 1, 3, 4, 3, 1, 2), then _19's get (1).
	slotPutMemoryTarget = 3 + 58 + 7 + 5 + 4 + 2 + 1 + 1 + 7 + 9 + 2 + 3 + 3 + 1 + 3 + 4 + 3 + 1 + 2 + 1
)

// lowMemoryWhenAway wires the window's events to the memory target policy; the page's
// input reaches it through API.PageInput.
func lowMemoryWhenAway(w *application.WebviewWindow) *memoryTarget {
	m := &memoryTarget{set: func(level uintptr) {
		application.InvokeAsync(func() { setMemoryTarget(webviewOf(w), level) })
	}}
	on := func(e events.WindowEventType, f func()) {
		w.OnWindowEvent(e, func(*application.WindowEvent) { f() })
	}
	on(events.Windows.WindowInactive, m.Inactive)
	on(events.Windows.WindowActive, m.Active)
	on(events.Windows.WindowClickActive, m.Active)
	on(events.Windows.WindowSetFocus, m.Active) // also when activation carried no event
	on(events.Windows.WindowMinimise, m.Minimised)
	on(events.Windows.WindowUnMinimise, m.Restored)
	on(events.Windows.WindowClosing, m.Stop)
	m.Input() // the idle wait starts with the window
	return m
}

// webviewOf is the window's ICoreWebView2: WebviewWindow.impl (*windowsWebviewWindow)
// .chromium (*edge.Chromium) .webview. nil when a field is missing or not set yet.
func webviewOf(w *application.WebviewWindow) unsafe.Pointer {
	v := reflect.ValueOf(w).Elem().FieldByName("impl")
	if !v.IsValid() || v.Kind() != reflect.Interface || v.IsNil() {
		return nil
	}
	v = v.Elem() // *windowsWebviewWindow
	for _, name := range []string{"chromium", "webview"} {
		if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
			return nil
		}
		if v = v.Elem().FieldByName(name); !v.IsValid() {
			return nil
		}
	}
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil
	}
	return v.UnsafePointer()
}

// comSlot is method i of a COM object's vtable.
func comSlot(obj unsafe.Pointer, i int) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	return *(*uintptr)(unsafe.Add(vtbl, i*int(unsafe.Sizeof(uintptr(0)))))
}

// setMemoryTarget asks the webview for ICoreWebView2_19 and sets its memory target; a
// WebView2 runtime without the interface (older than 1.0.2210) is left as it is.
func setMemoryTarget(wv unsafe.Pointer, level uintptr) bool {
	if wv == nil {
		return false
	}
	var wv19 unsafe.Pointer
	hr, _, _ := syscall.SyscallN(comSlot(wv, slotQueryInterface), uintptr(wv), uintptr(unsafe.Pointer(&iidWebView2_19)), uintptr(unsafe.Pointer(&wv19)))
	if hr != 0 || wv19 == nil {
		return false
	}
	defer syscall.SyscallN(comSlot(wv19, slotRelease), uintptr(wv19))
	hr, _, _ = syscall.SyscallN(comSlot(wv19, slotPutMemoryTarget), uintptr(wv19), level)
	return hr == 0
}
