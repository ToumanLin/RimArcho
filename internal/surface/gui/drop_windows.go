//go:build windows && (amd64 || arm64)

package gui

import (
	"os"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"golang.org/x/sys/windows"
)

// Files and links dropped on the window (16 §2.18 "Dropping files"). The window uses WebView2
// composition hosting (Snap Layouts, 16 §8.2), under which WebView2 registers no OLE drop
// target and Wails beta.28 forwards none, so nothing dropped reached the page. RimArcho
// registers its own IDropTarget on the window: a drag of files or a link emits "file-drag"
// (the page dims under its drop box) and a drop emits "files-dropped" with the paths, which
// of them are folders, and the link. Drags of anything else are refused (no drop cursor).
// POINTL travels in one register on amd64 and arm64, which the callbacks below rely on.

var (
	ole32   = windows.NewLazySystemDLL("ole32.dll")
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	procOleInitialize        = ole32.NewProc("OleInitialize")
	procRegisterDragDrop     = ole32.NewProc("RegisterDragDrop")
	procReleaseStgMedium     = ole32.NewProc("ReleaseStgMedium")
	procDragQueryFileW       = shell32.NewProc("DragQueryFileW")
	procGlobalLock           = kernel32.NewProc("GlobalLock")
	procMoveMemory           = kernel32.NewProc("RtlMoveMemory")
	procGlobalUnlock         = kernel32.NewProc("GlobalUnlock")
	procGlobalSize           = kernel32.NewProc("GlobalSize")
	procRegisterClipboardFmt = user32.NewProc("RegisterClipboardFormatW")
)

const (
	cfUnicodeText  = 13
	cfHDrop        = 15
	tymedHGlobal   = 1
	dropEffectCopy = 1
	eNoInterface   = 0x80004002
)

type formatEtc struct {
	cf     uint16
	ptd    uintptr
	aspect uint32
	lindex int32
	tymed  uint32
}

type stgMedium struct {
	tymed   uint32
	handle  uintptr
	release uintptr
}

type dropVtbl struct {
	queryInterface, addRef, release, dragEnter, dragOver, dragLeave, drop uintptr
}

// dropTarget is the COM object: its first word points at the vtable.
type dropTarget struct {
	vtbl    *dropVtbl
	refs    int32
	accept  bool
	drag    func(on bool)
	dropped func(d dropData)
}

var (
	iidIUnknown    = windows.GUID{Data1: 0, Data2: 0, Data3: 0, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIDropTarget = windows.GUID{Data1: 0x122, Data2: 0, Data3: 0, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	cfURL          uint16
	vtblOnce       sync.Once
	theVtbl        dropVtbl
	targets        []*dropTarget // kept alive while Windows holds them
)

// comObject is any COM object as Windows passes it: a pointer to its vtable.
type comObject struct{ vtbl *[16]uintptr }

// call calls method n of the object's vtable.
func (o *comObject) call(n int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[n], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}

// The callbacks take typed pointers (syscall.NewCallback passes them as they are), so no
// address is turned back into a pointer here.
func makeVtbl() {
	theVtbl = dropVtbl{
		queryInterface: syscall.NewCallback(func(t *dropTarget, riid *windows.GUID, ppv **dropTarget) uintptr {
			if *riid != iidIUnknown && *riid != iidIDropTarget {
				*ppv = nil
				return eNoInterface
			}
			*ppv = t
			t.refs++
			return 0
		}),
		addRef: syscall.NewCallback(func(t *dropTarget) uintptr {
			t.refs++
			return uintptr(t.refs)
		}),
		release: syscall.NewCallback(func(t *dropTarget) uintptr {
			if t.refs > 1 {
				t.refs--
			}
			return uintptr(t.refs)
		}),
		dragEnter: syscall.NewCallback(func(t *dropTarget, data *comObject, keys, pt uintptr, effect *uint32) uintptr {
			t.accept = offers(data, cfHDrop) || offers(data, cfURL) || isLink(dropText(data))
			setEffect(effect, t.accept)
			if t.accept {
				t.drag(true)
			}
			return 0
		}),
		dragOver: syscall.NewCallback(func(t *dropTarget, keys, pt uintptr, effect *uint32) uintptr {
			setEffect(effect, t.accept)
			return 0
		}),
		dragLeave: syscall.NewCallback(func(t *dropTarget) uintptr {
			if t.accept {
				t.drag(false)
			}
			t.accept = false
			return 0
		}),
		drop: syscall.NewCallback(func(t *dropTarget, data *comObject, keys, pt uintptr, effect *uint32) uintptr {
			d := dropData{Paths: dropPaths(data), Text: dropURL(data)}
			if d.Text == "" {
				if s := dropText(data); isLink(s) {
					d.Text = s
				}
			}
			for _, p := range d.Paths {
				fi, err := os.Stat(p)
				d.Dirs = append(d.Dirs, err == nil && fi.IsDir())
			}
			ok := t.accept && (len(d.Paths) > 0 || d.Text != "")
			setEffect(effect, ok)
			if t.accept {
				t.drag(false)
			}
			t.accept = false
			if ok {
				go t.dropped(d)
			}
			return 0
		}),
	}
}

func setEffect(effect *uint32, ok bool) {
	if effect == nil {
		return
	}
	*effect = 0
	if ok {
		*effect = dropEffectCopy
	}
}

// IDataObject: GetData is method 3, QueryGetData method 5.
func offers(data *comObject, cf uint16) bool {
	if data == nil || cf == 0 {
		return false
	}
	f := formatEtc{cf: cf, aspect: 1, lindex: -1, tymed: tymedHGlobal}
	return data.call(5, uintptr(unsafe.Pointer(&f))) == 0
}

func getData(data *comObject, cf uint16, use func(h uintptr)) {
	if !offers(data, cf) {
		return
	}
	f := formatEtc{cf: cf, aspect: 1, lindex: -1, tymed: tymedHGlobal}
	var m stgMedium
	if data.call(3, uintptr(unsafe.Pointer(&f)), uintptr(unsafe.Pointer(&m))) != 0 {
		return
	}
	defer procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&m)))
	if m.tymed == tymedHGlobal && m.handle != 0 {
		use(m.handle)
	}
}

func dropPaths(data *comObject) []string {
	var out []string
	getData(data, cfHDrop, func(h uintptr) {
		n, _, _ := procDragQueryFileW.Call(h, 0xFFFFFFFF, 0, 0)
		for i := uintptr(0); i < n && i < 256; i++ {
			size, _, _ := procDragQueryFileW.Call(h, i, 0, 0)
			buf := make([]uint16, size+1)
			procDragQueryFileW.Call(h, i, uintptr(unsafe.Pointer(&buf[0])), size+1)
			out = append(out, windows.UTF16ToString(buf))
		}
	})
	return out
}

// globalText copies a global memory block's UTF-16 text (at most 1 MiB).
func globalText(h uintptr) string {
	size, _, _ := procGlobalSize.Call(h)
	n := int(size / 2)
	if n <= 0 || n > 1<<19 {
		return ""
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(h)
	buf := make([]uint16, n)
	procMoveMemory.Call(uintptr(unsafe.Pointer(&buf[0])), p, uintptr(n*2))
	return windows.UTF16ToString(buf)
}

func dropURL(data *comObject) (s string) {
	getData(data, cfURL, func(h uintptr) { s = strings.TrimSpace(globalText(h)) })
	return s
}

func dropText(data *comObject) (s string) {
	getData(data, cfUnicodeText, func(h uintptr) { s = strings.TrimSpace(globalText(h)) })
	return s
}

// isLink: one line that is a web address (what add a subscription recognises)
func isLink(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \r\n\t") && (strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://"))
}

// wireNativeDrop registers the drop target once the window has its handle.
func wireNativeDrop(win *application.WebviewWindow, s *API) {
	var once sync.Once
	win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		once.Do(func() {
			application.InvokeAsync(func() {
				vtblOnce.Do(func() {
					makeVtbl()
					name, _ := windows.UTF16PtrFromString("UniformResourceLocatorW")
					r, _, _ := procRegisterClipboardFmt.Call(uintptr(unsafe.Pointer(name)))
					cfURL = uint16(r)
				})
				t := &dropTarget{vtbl: &theVtbl, refs: 1,
					drag:    func(on bool) { go s.send("file-drag", on) },
					dropped: func(d dropData) { s.send("files-dropped", d) }}
				targets = append(targets, t)
				procOleInitialize.Call(0)
				hwnd := uintptr(win.NativeWindow())
				if hr, _, _ := procRegisterDragDrop.Call(hwnd, uintptr(unsafe.Pointer(t))); hr != 0 {
					os.Stderr.WriteString("drop target: RegisterDragDrop failed\n")
				}
			})
		})
	})
}
