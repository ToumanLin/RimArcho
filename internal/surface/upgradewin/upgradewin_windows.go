// Package upgradewin shows a small native window while the database upgrades,
// before the GUI window opens (02 §7, 06 §8.1).
package upgradewin

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	gdi32            = windows.NewLazySystemDLL("gdi32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	comctl32         = windows.NewLazySystemDLL("comctl32.dll")
	registerClass    = user32.NewProc("RegisterClassExW")
	unregisterClass  = user32.NewProc("UnregisterClassW")
	createWindow     = user32.NewProc("CreateWindowExW")
	destroyWindow    = user32.NewProc("DestroyWindow")
	defWindowProc    = user32.NewProc("DefWindowProcW")
	getMessage       = user32.NewProc("GetMessageW")
	translateMessage = user32.NewProc("TranslateMessage")
	dispatchMessage  = user32.NewProc("DispatchMessageW")
	postMessage      = user32.NewProc("PostMessageW")
	postQuitMessage  = user32.NewProc("PostQuitMessage")
	sendMessage      = user32.NewProc("SendMessageW")
	setWindowText    = user32.NewProc("SetWindowTextW")
	showWindow       = user32.NewProc("ShowWindow")
	updateWindow     = user32.NewProc("UpdateWindow")
	workArea         = user32.NewProc("SystemParametersInfoW")
	adjustRect       = user32.NewProc("AdjustWindowRectEx")
	getDPI           = user32.NewProc("GetDpiForSystem")
	loadImage        = user32.NewProc("LoadImageW")
	loadCursor       = user32.NewProc("LoadCursorW")
	getModule        = kernel32.NewProc("GetModuleHandleW")
	createFont       = gdi32.NewProc("CreateFontW")
	deleteObject     = gdi32.NewProc("DeleteObject")
	initControls     = comctl32.NewProc("InitCommonControlsEx")
	callbackOnce     sync.Once
	callback         uintptr
	classID          atomic.Uint64
	windowsByHandle  sync.Map
)

const (
	wmUpdate = 0x8000 // WM_APP
	wmClose  = wmUpdate + 1
)

type rect struct{ left, top, right, bottom int32 }
type windowClass struct {
	size, style                        uint32
	proc                               uintptr
	classExtra, windowExtra            int32
	instance, icon, cursor, background uintptr
	menu, name                         *uint16
	smallIcon                          uintptr
}
type message struct {
	hwnd           uintptr
	id             uint32
	wparam, lparam uintptr
	time           uint32
	point          [2]int32
	private        uint32
}

// Window owns a native upgrade window. Its methods are safe on a nil receiver.
type Window struct {
	mu               sync.Mutex
	hwnd, label, bar uintptr
	text             string
	frac             float64
	pending, closing bool
	done             chan struct{}
}

// Open shows a title, one line of text and a progress bar, or returns nil on failure.
func Open(title, text string) *Window {
	w := &Window{text: text, done: make(chan struct{})}
	ready := make(chan *Window, 1)
	go w.run(title, ready)
	return <-ready
}

// Update sets the text and progress (0..1); a negative fraction keeps the bar.
// Calls from any goroutine coalesce until the window thread handles the update.
func (w *Window) Update(text string, frac float64) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing || w.hwnd == 0 {
		return
	}
	w.text = text
	if frac >= 0 {
		w.frac = min(frac, 1)
	}
	if !w.pending {
		posted, _, _ := postMessage.Call(w.hwnd, wmUpdate, 0, 0)
		w.pending = posted != 0
	}
}

// Close destroys the window and waits for its thread, with a two-second limit.
// Repeated calls are safe, including calls concurrent with Update.
func (w *Window) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.closing && w.hwnd != 0 {
		posted, _, _ := postMessage.Call(w.hwnd, wmClose, 0, 0)
		w.closing = posted != 0
	}
	w.mu.Unlock()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-w.done:
	case <-timer.C:
	}
}

func windowProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if value, ok := windowsByHandle.Load(hwnd); ok {
		w := value.(*Window)
		switch msg {
		case 0x0010: // WM_CLOSE: only Close may end the window.
			return 0
		case wmUpdate:
			w.mu.Lock()
			text, frac := w.text, w.frac
			w.pending = false
			w.mu.Unlock()
			if p, err := windows.UTF16PtrFromString(text); err == nil {
				setWindowText.Call(w.label, uintptr(unsafe.Pointer(p)))
			}
			sendMessage.Call(w.bar, 0x0402, uintptr(frac*1000), 0) // PBM_SETPOS
			return 0
		case wmClose:
			destroyWindow.Call(hwnd)
			return 0
		case 0x0002: // WM_DESTROY
			w.mu.Lock()
			w.hwnd = 0
			w.mu.Unlock()
			postQuitMessage.Call(0)
			return 0
		}
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wp, lp)
	return result
}

func (w *Window) run(title string, ready chan<- *Window) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var hwnd, font, instance uintptr
	var class *uint16
	opened := false
	defer func() {
		_ = recover() // A missing DLL or failed setup must not stop startup.
		w.mu.Lock()
		live := w.hwnd
		w.hwnd, w.closing = 0, true
		w.mu.Unlock()
		if live != 0 {
			destroyWindow.Call(live)
		}
		windowsByHandle.Delete(hwnd)
		if class != nil {
			unregisterClass.Call(uintptr(unsafe.Pointer(class)), instance)
		}
		if font != 0 {
			deleteObject.Call(font)
		}
		close(w.done)
		if !opened {
			ready <- nil
		}
	}()
	for _, proc := range []*windows.LazyProc{registerClass, unregisterClass, createWindow,
		destroyWindow, defWindowProc, getMessage, translateMessage, dispatchMessage,
		postMessage, postQuitMessage, sendMessage, setWindowText, showWindow,
		updateWindow, workArea, adjustRect, loadCursor, getModule, createFont, deleteObject, initControls} {
		if proc.Find() != nil {
			return
		}
	}
	callbackOnce.Do(func() { callback = windows.NewCallback(windowProc) })
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	textPtr, err := windows.UTF16PtrFromString(w.text)
	if err != nil {
		return
	}
	controls := [2]uint32{8, 0x20} // ICC_PROGRESS_CLASS
	if ok, _, _ := initControls.Call(uintptr(unsafe.Pointer(&controls))); ok == 0 {
		return
	}
	instance, _, _ = getModule.Call(0)
	if instance == 0 {
		return
	}
	dpi := 96
	if getDPI.Find() == nil {
		if value, _, _ := getDPI.Call(); value != 0 {
			dpi = int(value)
		}
	}
	scale := func(n int) int32 { return int32((n*dpi + 48) / 96) }
	var area rect
	if ok, _, _ := workArea.Call(0x30, 0, uintptr(unsafe.Pointer(&area)), 0); ok == 0 {
		return
	}
	bounds := rect{right: scale(420), bottom: scale(120)}
	const style = 0x00C00000 // WS_CAPTION, fixed size and no system menu.
	if ok, _, _ := adjustRect.Call(uintptr(unsafe.Pointer(&bounds)), style, 0, 0); ok == 0 {
		return
	}
	name, _ := windows.UTF16PtrFromString(fmt.Sprintf("RimArchoUpgrade_%d_%d", windows.GetCurrentProcessId(), classID.Add(1)))
	cursor, _, _ := loadCursor.Call(0, 32512) // IDC_ARROW
	wc := windowClass{size: uint32(unsafe.Sizeof(windowClass{})), proc: callback,
		instance: instance, cursor: cursor, background: 6, name: name} // COLOR_WINDOW+1
	if loadImage.Find() == nil {
		wc.icon, _, _ = loadImage.Call(instance, 3, 1, 0, 0, 0x8040) // IMAGE_ICON, LR_SHARED|LR_DEFAULTSIZE
	}
	if atom, _, _ := registerClass.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return
	}
	class = name
	width, height := bounds.right-bounds.left, bounds.bottom-bounds.top
	x, y := area.left+(area.right-area.left-width)/2, area.top+(area.bottom-area.top-height)/2
	hwnd, _, _ = createWindow.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(titlePtr)), style,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, instance, 0)
	if hwnd == 0 {
		return
	}
	w.hwnd = hwnd
	windowsByHandle.Store(hwnd, w)
	makeChild := func(kind string, text *uint16, y, height int) uintptr {
		kindPtr, _ := windows.UTF16PtrFromString(kind)
		child, _, _ := createWindow.Call(0, uintptr(unsafe.Pointer(kindPtr)), uintptr(unsafe.Pointer(text)), 0x50000000,
			uintptr(scale(20)), uintptr(scale(y)), uintptr(scale(380)), uintptr(scale(height)), hwnd, 0, instance, 0)
		return child
	}
	w.label, w.bar = makeChild("STATIC", textPtr, 22, 26), makeChild("msctls_progress32", nil, 64, 20)
	if w.label == 0 || w.bar == 0 {
		return
	}
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	fontHeight := -int32((9*dpi + 36) / 72)
	font, _, _ = createFont.Call(uintptr(fontHeight), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	if font == 0 {
		return
	}
	for _, child := range []uintptr{w.label, w.bar} {
		sendMessage.Call(child, 0x0030, font, 1)
	} // WM_SETFONT
	sendMessage.Call(w.bar, 0x0406, 0, 1000) // PBM_SETRANGE32
	showWindow.Call(hwnd, 5)
	if ok, _, _ := updateWindow.Call(hwnd); ok == 0 {
		return
	}
	opened = true
	ready <- w
	var msg message
	for {
		result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
