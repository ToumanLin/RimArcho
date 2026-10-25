//go:build windows

package accounts

// This host uses only native WebView2 COM APIs. There is deliberately no script
// execution, resource/body interception, web-message handler, host object or CDP.
import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func hideLoginHost(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

type comObject struct{ table *[80]uintptr }

func (o *comObject) call(slot int, args ...uintptr) uintptr {
	if o == nil {
		return 0x80004003
	}
	all := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	r, _, _ := syscall.SyscallN(o.table[slot], all...)
	return r
}
func (o *comObject) release() {
	if o != nil {
		o.call(2)
	}
}
func (o *comObject) query(id string) *comObject {
	g, _ := windows.GUIDFromString(id)
	var p *comObject
	o.call(0, uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&p)))
	return p
}
func comString(o *comObject, slot int) string {
	var p *uint16
	if o.call(slot, uintptr(unsafe.Pointer(&p))) != 0 {
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(p))
	return windows.UTF16PtrToString(p)
}
func comInt(o *comObject, slot int) int32 {
	var v int32
	o.call(slot, uintptr(unsafe.Pointer(&v)))
	return v
}
func wide(s string) *uint16   { p, _ := windows.UTF16PtrFromString(s); return p }
func ptr[T any](v *T) uintptr { return uintptr(unsafe.Pointer(v)) }

type callback struct{ table *[4]uintptr }

func handler(fn func(uintptr, *comObject) uintptr) *callback {
	c := &callback{}
	c.table = &[4]uintptr{
		windows.NewCallback(func(this *callback, _ *windows.GUID, out **callback) uintptr { *out = this; return 0 }),
		windows.NewCallback(func(*callback) uintptr { return 1 }), windows.NewCallback(func(*callback) uintptr { return 1 }),
		windows.NewCallback(func(_ *callback, a uintptr, b *comObject) uintptr { return fn(a, b) }),
	}
	return c
}

func runtimeDLL() (string, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		k, e := registry.OpenKey(root, `Software\Microsoft\EdgeUpdate\ClientState\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, registry.READ|registry.WOW64_32KEY)
		if e != nil {
			continue
		}
		folder, _, e := k.GetStringValue("EBWebView")
		k.Close()
		if e == nil && filepath.IsAbs(folder) {
			p := filepath.Join(folder, "EBWebView", arch, "EmbeddedBrowserWebView.dll")
			if _, e = os.Stat(p); e == nil {
				return p, nil
			}
		}
	}
	return "", errors.New("需要安装 WebView2 Runtime")
}

func (s *Session) login() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED) != nil {
		return ErrSession
	}
	defer windows.CoUninitialize()
	if err := privateDir(s.dir); err != nil {
		return ErrSession
	}
	profile := filepath.Join(s.dir, "webview-profile")
	// A previous crashed browser must not silently restore unrelated browser storage.
	if safeTree(profile) != nil || os.RemoveAll(profile) != nil {
		return ErrSession
	}
	if privateDir(profile) != nil {
		return ErrSession
	}
	defer os.RemoveAll(profile)
	dllPath, err := runtimeDLL()
	if err != nil {
		return err
	}
	dll, err := windows.LoadDLL(dllPath)
	if err != nil {
		return ErrSession
	}
	defer func() {
		if canUnload, e := dll.FindProc("DllCanUnloadNow"); e == nil {
			if hr, _, _ := canUnload.Call(); hr == 0 {
				_ = dll.Release()
			}
		}
	}()
	create, err := dll.FindProc("CreateWebViewEnvironmentWithOptionsInternal")
	if err != nil {
		return ErrSession
	}
	var controller, web, manager *comObject
	var callbacks []*callback
	var hwnd w32.HWND
	var failure error
	closing := false
	finish := func() {
		if controller != nil {
			controller.call(24)
		}
		w32.DestroyWindow(hwnd)
	}
	keep := func(fn func(uintptr, *comObject) uintptr) *callback {
		c := handler(fn)
		callbacks = append(callbacks, c)
		return c
	}
	cookieDone := keep(func(code uintptr, list *comObject) uintptr {
		if code != 0 || list == nil {
			failure = ErrSession
			finish()
			return 0
		}
		count := comInt(list, 3)
		var cookies []*http.Cookie
		for i := int32(0); i < count; i++ {
			var c *comObject
			if list.call(4, uintptr(i), ptr(&c)) != 0 {
				continue
			}
			v := &http.Cookie{Name: comString(c, 3), Value: comString(c, 4), Domain: comString(c, 6), Path: comString(c, 7), HttpOnly: comInt(c, 10) != 0, Secure: comInt(c, 14) != 0}
			var expiry float64
			c.call(8, ptr(&expiry))
			if comInt(c, 16) == 0 && expiry > 0 {
				v.Expires = time.Unix(int64(expiry), 0).UTC()
			}
			switch comInt(c, 12) {
			case 0:
				v.SameSite = http.SameSiteNoneMode
			case 1:
				v.SameSite = http.SameSiteLaxMode
			case 2:
				v.SameSite = http.SameSiteStrictMode
			}
			cookies = append(cookies, v)
			c.release()
		}
		failure = s.capture(cookies)
		finish()
		return 0
	})
	class := wide("RimArchoLoversLabLogin")
	proc := windows.NewCallback(func(h w32.HWND, msg uint32, wp, lp uintptr) uintptr {
		switch msg {
		case w32.WM_SIZE:
			if controller != nil {
				controller.call(6, ptr(w32.GetClientRect(h)))
			}
		case w32.WM_CLOSE:
			if !closing {
				closing = true
				if manager != nil {
					if manager.call(5, ptr(wide("")), ptr(cookieDone)) != 0 {
						failure = ErrSession
						finish()
					}
				} else {
					finish()
				}
			}
			return 0
		case w32.WM_DESTROY:
			w32.PostQuitMessage(0)
			return 0
		}
		return w32.DefWindowProc(h, msg, wp, lp)
	})
	instance := w32.GetModuleHandle("")
	wc := w32.WNDCLASSEX{WndProc: proc, ClassName: class, Instance: instance}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if w32.RegisterClassEx(&wc) == 0 {
		return ErrSession
	}
	hwnd = w32.CreateWindowEx(0, class, wide("LoversLab — 登录完成后关闭此窗口保存会话"), w32.WS_OVERLAPPEDWINDOW|w32.WS_VISIBLE, 100, 100, 1024, 780, 0, 0, instance, nil)
	if hwnd == 0 {
		return ErrSession
	}
	controllerDone := keep(func(code uintptr, c *comObject) uintptr {
		if code != 0 || c == nil {
			failure = ErrSession
			finish()
			return 0
		}
		controller = c
		c.call(1)
		if c.call(25, ptr(&web)) != 0 {
			failure = ErrSession
			finish()
			return 0
		}
		var settings *comObject
		web.call(3, ptr(&settings))
		secure := settings.query("{cb56846c-4168-4d53-b04f-03b6d6796ff2}")
		settings.release()
		if secure == nil {
			failure = ErrSession
			finish()
			return 0
		}
		defer secure.release()
		for _, slot := range []int{6, 12, 14, 16, 24, 26, 28} {
			if secure.call(slot, 0) != 0 {
				failure = ErrSession
				finish()
				return 0
			}
		}
		v2 := web.query("{9E8F0CF8-E670-4B5E-B2BC-73E061E3184C}")
		if v2 == nil {
			failure = ErrSession
			finish()
			return 0
		}
		v2.call(66, ptr(&manager))
		v2.release()
		if manager == nil {
			failure = ErrSession
			finish()
			return 0
		}
		// Restore only cookies already admitted by the service boundary.
		for _, v := range s.cookies {
			var cookie *comObject
			if manager.call(3, ptr(wide(v.Name)), ptr(wide(v.Value)), ptr(wide(v.Domain)), ptr(wide(v.Path)), ptr(&cookie)) == 0 {
				cookie.call(15, 1)
				if v.HttpOnly {
					cookie.call(11, 1)
				}
				sameSite := uintptr(1)
				if v.SameSite == http.SameSiteNoneMode {
					sameSite = 0
				}
				if v.SameSite == http.SameSiteStrictMode {
					sameSite = 2
				}
				cookie.call(13, sameSite)
				// Expires uses the x64 floating-point ABI; SyscallN mirrors argument bits to XMM.
				if !v.Expires.IsZero() {
					cookie.call(9, uintptr(math.Float64bits(float64(v.Expires.Unix()))))
				}
				manager.call(6, ptr(cookie))
				cookie.release()
			}
		}
		var token int64
		v4 := web.query("{20d02d59-6df2-42dc-bd06-f98a694b1302}")
		if v4 == nil {
			failure = ErrSession
			finish()
			return 0
		}
		// ICoreWebView2_4 inherits the base, _2 and _3 slots. Login hosts do
		// not download files into the player's ordinary Downloads folder.
		download := keep(func(_ uintptr, args *comObject) uintptr { args.call(5, 1); args.call(9, 1); return 0 })
		if v4.call(75, ptr(download), ptr(&token)) != 0 {
			v4.release()
			failure = ErrSession
			finish()
			return 0
		}
		v4.release()
		nav := keep(func(_ uintptr, args *comObject) uintptr {
			raw := comString(args, 3)
			u, e := url.Parse(raw)
			if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !SiteHost(u.Hostname()) {
				args.call(8, 1)
			}
			return 0
		})
		web.call(7, ptr(nav), ptr(&token))
		frame := keep(func(_ uintptr, args *comObject) uintptr {
			u, e := url.Parse(comString(args, 3))
			if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || (!SiteHost(u.Hostname()) && u.Hostname() != "challenges.cloudflare.com") {
				args.call(8, 1)
			}
			return 0
		})
		web.call(17, ptr(frame), ptr(&token))
		popup := keep(func(_ uintptr, args *comObject) uintptr { args.call(6, 1); return 0 })
		web.call(44, ptr(popup), ptr(&token))
		permission := keep(func(_ uintptr, args *comObject) uintptr { args.call(7, 2); return 0 })
		web.call(23, ptr(permission), ptr(&token))
		controller.call(6, ptr(w32.GetClientRect(hwnd)))
		controller.call(4, 1)
		web.call(5, ptr(wide(Site+"login/")))
		return 0
	})
	envDone := keep(func(code uintptr, env *comObject) uintptr {
		if code != 0 || env == nil {
			failure = ErrSession
			finish()
			return 0
		}
		if env.call(3, uintptr(hwnd), ptr(controllerDone)) != 0 {
			failure = ErrSession
			finish()
		}
		return 0
	})
	// Direct runtime entry point avoids environment/registry loader overrides. There
	// are no extra browser arguments, remote-debugging ports or runtime injection.
	hr, _, _ := create.Call(1, 0, ptr(wide(profile)), 0, ptr(envDone))
	if hr != 0 {
		w32.DestroyWindow(hwnd)
		return ErrSession
	}
	var msg w32.MSG
	for w32.GetMessage(&msg, 0, 0, 0) > 0 {
		w32.TranslateMessage(&msg)
		w32.DispatchMessage(&msg)
	}
	manager.release()
	web.release()
	controller.release()
	runtime.KeepAlive(callbacks)
	// The runtime can outlive Controller.Close. Wait for its own process to exit
	// before removing the ephemeral profile; never claim a failed cleanup succeeded.
	for i := 0; i < 50; i++ {
		if os.RemoveAll(profile) == nil {
			return failure
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ErrSession
}
