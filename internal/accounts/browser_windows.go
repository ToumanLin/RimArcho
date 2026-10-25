//go:build windows

package accounts

// This host uses only native WebView2 COM APIs. There is deliberately no script
// execution, resource/body interception, web-message handler, host object or CDP.
import (
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
)

func hideLoginHost(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

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
		o.call(2) // IUnknown.Release
	}
}
func (o *comObject) query(id string) *comObject {
	g, _ := windows.GUIDFromString(id)
	var p *comObject
	o.call(0, uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&p))) // IUnknown.QueryInterface
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

func (s *Session) login(check bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// S_FALSE means this thread already has the requested COM apartment.
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		return helperFailure("COM initialize failed", uintptr(err.(syscall.Errno)))
	}
	defer windows.CoUninitialize()
	if err := privateDir(s.dir); err != nil {
		return helperFailure("login window failed or session cleanup failed", 0)
	}
	profile := filepath.Join(s.dir, "webview-profile")
	// A previous crashed browser must not silently restore unrelated browser storage.
	if safeTree(profile) != nil || os.RemoveAll(profile) != nil {
		return helperFailure("login window failed or session cleanup failed", 0)
	}
	if privateDir(profile) != nil {
		return helperFailure("login window failed or session cleanup failed", 0)
	}
	defer os.RemoveAll(profile)
	dllPath, err := runtimeDLL()
	if err != nil {
		return err
	}
	dll, err := windows.LoadDLL(dllPath)
	if err != nil {
		return helperFailure("runtime load failed", 0)
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
		return helperFailure("runtime entry point not found", 0)
	}
	var controller, web, manager *comObject
	var callbacks []*callback
	var hwnd w32.HWND
	var failure error
	closing := false
	finish := func() {
		if controller != nil {
			controller.call(24) // ICoreWebView2Controller.Close
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
		count := comInt(list, 3) // ICoreWebView2CookieList.get_Count
		var cookies []*http.Cookie
		for i := int32(0); i < count; i++ {
			var c *comObject
			if list.call(4, uintptr(i), ptr(&c)) != 0 { // ICoreWebView2CookieList.GetValueAtIndex
				continue
			}
			v := &http.Cookie{
				Name:     comString(c, 3),    // ICoreWebView2Cookie.get_Name
				Value:    comString(c, 4),    // get_Value
				Domain:   comString(c, 6),    // get_Domain
				Path:     comString(c, 7),    // get_Path
				HttpOnly: comInt(c, 10) != 0, // get_IsHttpOnly
				Secure:   comInt(c, 14) != 0, // get_IsSecure
			}
			var expiry float64
			c.call(8, ptr(&expiry))               // ICoreWebView2Cookie.get_Expires
			if comInt(c, 16) == 0 && expiry > 0 { // ICoreWebView2Cookie.get_IsSession
				v.Expires = time.Unix(int64(expiry), 0).UTC()
			}
			switch comInt(c, 12) { // ICoreWebView2Cookie.get_SameSite
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
				controller.call(6, ptr(w32.GetClientRect(h))) // ICoreWebView2Controller.put_Bounds
			}
		case w32.WM_CLOSE:
			if !closing {
				closing = true
				if check {
					finish()
				} else if manager != nil {
					if manager.call(5, ptr(wide("")), ptr(cookieDone)) != 0 { // ICoreWebView2CookieManager.GetCookies
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
		return helperFailure("window class registration failed", 0)
	}
	hwnd = w32.CreateWindowEx(0, class, wide("LoversLab — 登录完成后关闭此窗口保存会话"), w32.WS_OVERLAPPEDWINDOW|w32.WS_VISIBLE, 100, 100, 1024, 780, 0, 0, instance, nil)
	if hwnd == 0 {
		return helperFailure("window create failed", 0)
	}
	// Consume any inherited STARTF_USESHOWWINDOW before the controller is ready.
	w32.ShowWindow(hwnd, w32.SW_SHOW)
	controllerDone := keep(func(code uintptr, c *comObject) uintptr {
		if code != 0 || c == nil {
			failure = helperFailure("controller create failed", code)
			finish()
			return 0
		}
		controller = c
		c.call(1)                                 // IUnknown.AddRef
		if hr := c.call(25, ptr(&web)); hr != 0 { // ICoreWebView2Controller.get_CoreWebView2
			failure = helperFailure("webview interface failed", hr)
			finish()
			return 0
		}
		var settings *comObject
		web.call(3, ptr(&settings)) // ICoreWebView2.get_Settings
		secure := settings.query("{cb56846c-4168-4d53-b04f-03b6d6796ff2}")
		settings.release()
		if secure == nil {
			failure = helperFailure("settings interface not available", 0)
			finish()
			return 0
		}
		defer secure.release()
		// ICoreWebView2Settings4 setters, checked against WebView2.h.
		for _, slot := range []int{
			6,  // put_IsWebMessageEnabled
			12, // put_AreDevToolsEnabled
			14, // put_AreDefaultContextMenusEnabled
			16, // put_AreHostObjectsAllowed
			24, // put_AreBrowserAcceleratorKeysEnabled
			26, // put_IsPasswordAutosaveEnabled
			28, // put_IsGeneralAutofillEnabled
		} {
			if hr := secure.call(slot, 0); hr != 0 {
				failure = helperFailure("settings apply failed", hr)
				finish()
				return 0
			}
		}
		v2 := web.query("{9E8F0CF8-E670-4B5E-B2BC-73E061E3184C}")
		if v2 == nil {
			failure = helperFailure("cookie manager interface not available", 0)
			finish()
			return 0
		}
		v2.call(66, ptr(&manager)) // ICoreWebView2_2.get_CookieManager
		v2.release()
		if manager == nil {
			failure = helperFailure("cookie manager not available", 0)
			finish()
			return 0
		}
		// Restore only cookies already admitted by the service boundary.
		for _, v := range s.cookies {
			var cookie *comObject
			if manager.call(3, ptr(wide(v.Name)), ptr(wide(v.Value)), ptr(wide(v.Domain)), ptr(wide(v.Path)), ptr(&cookie)) == 0 { // ICoreWebView2CookieManager.CreateCookie
				cookie.call(15, 1) // ICoreWebView2Cookie.put_IsSecure
				if v.HttpOnly {
					cookie.call(11, 1) // ICoreWebView2Cookie.put_IsHttpOnly
				}
				sameSite := uintptr(1)
				if v.SameSite == http.SameSiteNoneMode {
					sameSite = 0
				}
				if v.SameSite == http.SameSiteStrictMode {
					sameSite = 2
				}
				cookie.call(13, sameSite) // ICoreWebView2Cookie.put_SameSite
				// Expires uses the x64 floating-point ABI; SyscallN mirrors argument bits to XMM.
				if !v.Expires.IsZero() {
					cookie.call(9, uintptr(math.Float64bits(float64(v.Expires.Unix())))) // ICoreWebView2Cookie.put_Expires
				}
				manager.call(6, ptr(cookie)) // ICoreWebView2CookieManager.AddOrUpdateCookie
				cookie.release()
			}
		}
		setup := func(stage string, hr uintptr) bool {
			if hr == 0 {
				return true
			}
			failure = helperFailure(stage, hr)
			finish()
			return false
		}
		var token int64
		v4 := web.query("{20d02d59-6df2-42dc-bd06-f98a694b1302}")
		if v4 == nil {
			failure = helperFailure("download interface not available", 0)
			finish()
			return 0
		}
		// ICoreWebView2_4 inherits the base, _2 and _3 slots. Login hosts do
		// not download files into the player's ordinary Downloads folder.
		download := keep(func(_ uintptr, args *comObject) uintptr {
			args.call(5, 1) // ICoreWebView2DownloadStartingEventArgs.put_Cancel
			args.call(9, 1) // put_Handled
			return 0
		})
		if hr := v4.call(75, ptr(download), ptr(&token)); hr != 0 { // ICoreWebView2_4.add_DownloadStarting
			v4.release()
			failure = helperFailure("download handler failed", hr)
			finish()
			return 0
		}
		v4.release()
		nav := keep(func(_ uintptr, args *comObject) uintptr {
			raw := comString(args, 3) // ICoreWebView2NavigationStartingEventArgs.get_Uri
			u, e := url.Parse(raw)
			if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !SiteHost(u.Hostname()) {
				args.call(8, 1) // ICoreWebView2NavigationStartingEventArgs.put_Cancel
			}
			return 0
		})
		if !setup("navigation handler failed", web.call(7, ptr(nav), ptr(&token))) { // ICoreWebView2.add_NavigationStarting
			return 0
		}
		frame := keep(func(_ uintptr, args *comObject) uintptr {
			u, e := url.Parse(comString(args, 3)) // ICoreWebView2NavigationStartingEventArgs.get_Uri
			if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || (!SiteHost(u.Hostname()) && u.Hostname() != "challenges.cloudflare.com") {
				args.call(8, 1) // ICoreWebView2NavigationStartingEventArgs.put_Cancel
			}
			return 0
		})
		if !setup("frame handler failed", web.call(17, ptr(frame), ptr(&token))) { // ICoreWebView2.add_FrameNavigationStarting
			return 0
		}
		popup := keep(func(_ uintptr, args *comObject) uintptr {
			args.call(6, 1) // ICoreWebView2NewWindowRequestedEventArgs.put_Handled
			return 0
		})
		if !setup("popup handler failed", web.call(44, ptr(popup), ptr(&token))) { // ICoreWebView2.add_NewWindowRequested
			return 0
		}
		permission := keep(func(_ uintptr, args *comObject) uintptr {
			args.call(7, 2) // ICoreWebView2PermissionRequestedEventArgs.put_State (DENY)
			return 0
		})
		if !setup("permission handler failed", web.call(23, ptr(permission), ptr(&token))) { // ICoreWebView2.add_PermissionRequested
			return 0
		}
		if !setup("controller bounds failed", controller.call(6, ptr(w32.GetClientRect(hwnd)))) { // ICoreWebView2Controller.put_Bounds
			return 0
		}
		if !setup("controller visibility failed", controller.call(4, 1)) { // ICoreWebView2Controller.put_IsVisible
			return 0
		}
		if !setup("navigation failed", web.call(5, ptr(wide(Site+"login/")))) { // ICoreWebView2.Navigate
			return 0
		}
		w32.ShowWindow(hwnd, w32.SW_SHOW)
		writeHelperLog(s.dir, helperFailure("login window ready", 0))
		return 0
	})
	envDone := keep(func(code uintptr, unknown *comObject) uintptr {
		if code != 0 || unknown == nil {
			failure = helperFailure("environment create failed", code)
			finish()
			return 0
		}
		// Wails' loader queries the environment interface on the returned IUnknown.
		environment := unknown.query("{b96d755e-0319-4e92-a296-23436f46a1fc}")
		if environment == nil {
			failure = helperFailure("environment interface not available", 0)
			finish()
			return 0
		}
		defer environment.release()
		if hr := environment.call(3, uintptr(hwnd), ptr(controllerDone)); hr != 0 { // ICoreWebView2Environment.CreateCoreWebView2Controller
			failure = helperFailure("controller create failed", hr)
			finish()
		}
		return 0
	})
	// Same five-argument ABI as Wails' pure-Go webviewloader: unknown=1,
	// installed runtime=0, user-data folder, environment options, completion handler.
	// See webviewloader_LICENSE for the upstream loader's ISC notice.
	options := newEnvironmentOptions()
	hr, _, _ := create.Call(1, 0, ptr(wide(profile)), ptr(&options.primary), ptr(envDone))
	if hr != 0 {
		w32.DestroyWindow(hwnd)
		return helperFailure("environment create failed", hr)
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
	runtime.KeepAlive(options)
	// The runtime can outlive Controller.Close. Wait for its own process to exit
	// before removing the ephemeral profile; never claim a failed cleanup succeeded.
	for i := 0; i < 50; i++ {
		if os.RemoveAll(profile) == nil {
			return failure
		}
		time.Sleep(100 * time.Millisecond)
	}
	return helperFailure("login window failed or session cleanup failed", 0)
}
