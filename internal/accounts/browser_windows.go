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

// capture is called only by the native CookieManager completion handler.
func (s *Session) capture(cookies []*http.Cookie) error {
	s.accept(cookies)
	s.status.State = "login required"
	return s.save()
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
	profile, err := s.freshProfile()
	if err != nil {
		return err
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
	defer unloadRuntime(dll)
	create, err := dll.FindProc("CreateWebViewEnvironmentWithOptionsInternal")
	if err != nil {
		return helperFailure("runtime entry point not found", 0)
	}
	w := &loginWindow{s: s, check: check}
	w.cookieDone = w.keep(w.onCookies)
	if err := w.createWindow(); err != nil {
		return err
	}
	w.controllerDone = w.keep(w.onController)
	envDone := w.keep(w.onEnvironment)
	// Same five-argument ABI as Wails' pure-Go webviewloader: unknown=1,
	// installed runtime=0, user-data folder, environment options, completion handler.
	// See webviewloader_LICENSE for the upstream loader's ISC notice.
	options := newEnvironmentOptions()
	hr, _, _ := create.Call(1, 0, ptr(wide(profile)), ptr(&options.primary), ptr(envDone))
	if hr != 0 {
		w32.DestroyWindow(w.hwnd)
		return helperFailure("environment create failed", hr)
	}
	pumpMessages()
	w.manager.release()
	w.web.release()
	w.controller.release()
	runtime.KeepAlive(w.callbacks)
	runtime.KeepAlive(options)
	return removeProfile(profile, w.failure)
}

// freshProfile returns an empty private WebView2 profile folder in the session folder.
func (s *Session) freshProfile() (string, error) {
	if err := privateDir(s.dir); err != nil {
		return "", helperFailure("login window failed or session cleanup failed", 0)
	}
	profile := filepath.Join(s.dir, "webview-profile")
	// A previous crashed browser must not silently restore unrelated browser storage.
	if safeTree(profile) != nil || os.RemoveAll(profile) != nil {
		return "", helperFailure("login window failed or session cleanup failed", 0)
	}
	if privateDir(profile) != nil {
		return "", helperFailure("login window failed or session cleanup failed", 0)
	}
	return profile, nil
}

// unloadRuntime releases the WebView2 loader DLL once it says it can be unloaded.
func unloadRuntime(dll *windows.DLL) {
	if canUnload, e := dll.FindProc("DllCanUnloadNow"); e == nil {
		if hr, _, _ := canUnload.Call(); hr == 0 {
			_ = dll.Release()
		}
	}
}

// pumpMessages runs the thread's message loop until WM_QUIT.
func pumpMessages() {
	var msg w32.MSG
	for w32.GetMessage(&msg, 0, 0, 0) > 0 {
		w32.TranslateMessage(&msg)
		w32.DispatchMessage(&msg)
	}
}

// removeProfile deletes the ephemeral profile and returns failure, or a cleanup failure if it stays.
// The runtime can outlive Controller.Close. Wait for its own process to exit
// before removing the ephemeral profile; never claim a failed cleanup succeeded.
func removeProfile(profile string, failure error) error {
	for i := 0; i < 50; i++ {
		if os.RemoveAll(profile) == nil {
			return failure
		}
		time.Sleep(100 * time.Millisecond)
	}
	return helperFailure("login window failed or session cleanup failed", 0)
}

// loginWindow is the state one login window's window procedure and WebView2 handlers share.
type loginWindow struct {
	s                          *Session
	check                      bool
	controller, web, manager   *comObject
	callbacks                  []*callback
	hwnd                       w32.HWND
	failure                    error
	closing                    bool
	cookieDone, controllerDone *callback
}

// finish closes the controller and destroys the window, which ends the message loop.
func (w *loginWindow) finish() {
	if w.controller != nil {
		w.controller.call(24) // ICoreWebView2Controller.Close
	}
	w32.DestroyWindow(w.hwnd)
}

// fail records err as the login's result and closes the window.
func (w *loginWindow) fail(err error) uintptr {
	w.failure = err
	w.finish()
	return 0
}

// keep wraps fn as a COM completion or event handler and keeps it alive for the login.
func (w *loginWindow) keep(fn func(uintptr, *comObject) uintptr) *callback {
	c := handler(fn)
	w.callbacks = append(w.callbacks, c)
	return c
}

// onCookies receives the cookie list on close, captures it into the session and closes the window.
func (w *loginWindow) onCookies(code uintptr, list *comObject) uintptr {
	if code != 0 || list == nil {
		return w.fail(ErrSession)
	}
	count := comInt(list, 3) // ICoreWebView2CookieList.get_Count
	var cookies []*http.Cookie
	for i := int32(0); i < count; i++ {
		var c *comObject
		if list.call(4, uintptr(i), ptr(&c)) != 0 { // ICoreWebView2CookieList.GetValueAtIndex
			continue
		}
		cookies = append(cookies, readCookie(c))
		c.release()
	}
	return w.fail(w.s.capture(cookies))
}

// readCookie copies one ICoreWebView2Cookie into an http.Cookie.
func readCookie(c *comObject) *http.Cookie {
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
	v.SameSite = cookieSameSite(comInt(c, 12)) // ICoreWebView2Cookie.get_SameSite
	return v
}

// cookieSameSite maps WebView2's SameSite kind to http.SameSite; unknown kinds stay unset.
func cookieSameSite(code int32) http.SameSite {
	switch code {
	case 0:
		return http.SameSiteNoneMode
	case 1:
		return http.SameSiteLaxMode
	case 2:
		return http.SameSiteStrictMode
	}
	return 0
}

// webViewSameSite maps http.SameSite to WebView2's SameSite kind, Lax unless None or Strict.
func webViewSameSite(mode http.SameSite) uintptr {
	sameSite := uintptr(1)
	if mode == http.SameSiteNoneMode {
		sameSite = 0
	}
	if mode == http.SameSiteStrictMode {
		sameSite = 2
	}
	return sameSite
}

// createWindow registers the window class and creates and shows the login window.
func (w *loginWindow) createWindow() error {
	class := wide("RimArchoLoversLabLogin")
	proc := windows.NewCallback(w.windowProc)
	instance := w32.GetModuleHandle("")
	wc := w32.WNDCLASSEX{WndProc: proc, ClassName: class, Instance: instance}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if w32.RegisterClassEx(&wc) == 0 {
		return helperFailure("window class registration failed", 0)
	}
	w.hwnd = w32.CreateWindowEx(0, class, wide(w.s.title), w32.WS_OVERLAPPEDWINDOW|w32.WS_VISIBLE, 100, 100, 1024, 780, 0, 0, instance, nil)
	if w.hwnd == 0 {
		return helperFailure("window create failed", 0)
	}
	// Consume any inherited STARTF_USESHOWWINDOW before the controller is ready.
	w32.ShowWindow(w.hwnd, w32.SW_SHOW)
	return nil
}

// windowProc resizes the browser with the window, reads cookies on close and quits on destroy.
func (w *loginWindow) windowProc(h w32.HWND, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case w32.WM_SIZE:
		if w.controller != nil {
			w.controller.call(6, ptr(w32.GetClientRect(h))) // ICoreWebView2Controller.put_Bounds
		}
	case w32.WM_CLOSE:
		w.onClose()
		return 0
	case w32.WM_DESTROY:
		w32.PostQuitMessage(0)
		return 0
	}
	return w32.DefWindowProc(h, msg, wp, lp)
}

// onClose closes a check window at once; a login window first asks for its cookies.
func (w *loginWindow) onClose() {
	if w.closing {
		return
	}
	w.closing = true
	if w.check {
		w.finish()
	} else if w.manager != nil {
		if w.manager.call(5, ptr(wide("")), ptr(w.cookieDone)) != 0 { // ICoreWebView2CookieManager.GetCookies
			w.fail(ErrSession)
		}
	} else {
		w.finish()
	}
}

// onEnvironment asks the new WebView2 environment for a controller on the login window.
func (w *loginWindow) onEnvironment(code uintptr, unknown *comObject) uintptr {
	if code != 0 || unknown == nil {
		return w.fail(helperFailure("environment create failed", code))
	}
	// Wails' loader queries the environment interface on the returned IUnknown.
	environment := unknown.query("{b96d755e-0319-4e92-a296-23436f46a1fc}")
	if environment == nil {
		return w.fail(helperFailure("environment interface not available", 0))
	}
	defer environment.release()
	if hr := environment.call(3, uintptr(w.hwnd), ptr(w.controllerDone)); hr != 0 { // ICoreWebView2Environment.CreateCoreWebView2Controller
		w.fail(helperFailure("controller create failed", hr))
	}
	return 0
}

// onController locks down the new browser, restores the session's cookies, adds the handlers and opens the login page.
func (w *loginWindow) onController(code uintptr, c *comObject) uintptr {
	if code != 0 || c == nil {
		return w.fail(helperFailure("controller create failed", code))
	}
	w.controller = c
	c.call(1)                                   // IUnknown.AddRef
	if hr := c.call(25, ptr(&w.web)); hr != 0 { // ICoreWebView2Controller.get_CoreWebView2
		return w.fail(helperFailure("webview interface failed", hr))
	}
	var settings *comObject
	w.web.call(3, ptr(&settings)) // ICoreWebView2.get_Settings
	secure := settings.query("{cb56846c-4168-4d53-b04f-03b6d6796ff2}")
	settings.release()
	if secure == nil {
		return w.fail(helperFailure("settings interface not available", 0))
	}
	defer secure.release()
	if err := disableBrowserFeatures(secure); err != nil {
		return w.fail(err)
	}
	if err := w.openCookieManager(); err != nil {
		return w.fail(err)
	}
	w.restoreCookies()
	if !w.addHandlers() {
		return 0
	}
	if !w.setup("controller bounds failed", w.controller.call(6, ptr(w32.GetClientRect(w.hwnd)))) { // ICoreWebView2Controller.put_Bounds
		return 0
	}
	if !w.setup("controller visibility failed", w.controller.call(4, 1)) { // ICoreWebView2Controller.put_IsVisible
		return 0
	}
	if !w.setup("navigation failed", w.web.call(5, ptr(wide(Site+"login/")))) { // ICoreWebView2.Navigate
		return 0
	}
	w32.ShowWindow(w.hwnd, w32.SW_SHOW)
	writeHelperLog(w.s.dir, helperFailure("login window ready", 0))
	return 0
}

// disableBrowserFeatures turns off web messages, dev tools, context menus, host objects, accelerator keys and autofill.
func disableBrowserFeatures(secure *comObject) error {
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
			return helperFailure("settings apply failed", hr)
		}
	}
	return nil
}

// openCookieManager gets the browser's cookie manager.
func (w *loginWindow) openCookieManager() error {
	v2 := w.web.query("{9E8F0CF8-E670-4B5E-B2BC-73E061E3184C}")
	if v2 == nil {
		return helperFailure("cookie manager interface not available", 0)
	}
	v2.call(66, ptr(&w.manager)) // ICoreWebView2_2.get_CookieManager
	v2.release()
	if w.manager == nil {
		return helperFailure("cookie manager not available", 0)
	}
	return nil
}

// restoreCookies adds the session's cookies to the browser.
// Restore only cookies already admitted by the service boundary.
func (w *loginWindow) restoreCookies() {
	for _, v := range w.s.cookies {
		var cookie *comObject
		if w.manager.call(3, ptr(wide(v.Name)), ptr(wide(v.Value)), ptr(wide(v.Domain)), ptr(wide(v.Path)), ptr(&cookie)) == 0 { // ICoreWebView2CookieManager.CreateCookie
			cookie.call(15, 1) // ICoreWebView2Cookie.put_IsSecure
			if v.HttpOnly {
				cookie.call(11, 1) // ICoreWebView2Cookie.put_IsHttpOnly
			}
			cookie.call(13, webViewSameSite(v.SameSite)) // ICoreWebView2Cookie.put_SameSite
			// Expires uses the x64 floating-point ABI; SyscallN mirrors argument bits to XMM.
			if !v.Expires.IsZero() {
				cookie.call(9, uintptr(math.Float64bits(float64(v.Expires.Unix())))) // ICoreWebView2Cookie.put_Expires
			}
			w.manager.call(6, ptr(cookie)) // ICoreWebView2CookieManager.AddOrUpdateCookie
			cookie.release()
		}
	}
}

// setup reports whether a setup call succeeded, failing the login with stage if not.
func (w *loginWindow) setup(stage string, hr uintptr) bool {
	if hr == 0 {
		return true
	}
	w.fail(helperFailure(stage, hr))
	return false
}

// addHandlers cancels downloads, off-site navigations, popups and permission requests; false means the login failed.
func (w *loginWindow) addHandlers() bool {
	var token int64
	v4 := w.web.query("{20d02d59-6df2-42dc-bd06-f98a694b1302}")
	if v4 == nil {
		w.fail(helperFailure("download interface not available", 0))
		return false
	}
	// ICoreWebView2_4 inherits the base, _2 and _3 slots. Login hosts do
	// not download files into the player's ordinary Downloads folder.
	download := w.keep(func(_ uintptr, args *comObject) uintptr {
		args.call(5, 1) // ICoreWebView2DownloadStartingEventArgs.put_Cancel
		args.call(9, 1) // put_Handled
		return 0
	})
	if hr := v4.call(75, ptr(download), ptr(&token)); hr != 0 { // ICoreWebView2_4.add_DownloadStarting
		v4.release()
		w.fail(helperFailure("download handler failed", hr))
		return false
	}
	v4.release()
	nav := w.keep(func(_ uintptr, args *comObject) uintptr {
		if blockedNavigation(comString(args, 3), false) { // ICoreWebView2NavigationStartingEventArgs.get_Uri
			args.call(8, 1) // ICoreWebView2NavigationStartingEventArgs.put_Cancel
		}
		return 0
	})
	if !w.setup("navigation handler failed", w.web.call(7, ptr(nav), ptr(&token))) { // ICoreWebView2.add_NavigationStarting
		return false
	}
	frame := w.keep(func(_ uintptr, args *comObject) uintptr {
		if blockedNavigation(comString(args, 3), true) { // ICoreWebView2NavigationStartingEventArgs.get_Uri
			args.call(8, 1) // ICoreWebView2NavigationStartingEventArgs.put_Cancel
		}
		return 0
	})
	if !w.setup("frame handler failed", w.web.call(17, ptr(frame), ptr(&token))) { // ICoreWebView2.add_FrameNavigationStarting
		return false
	}
	popup := w.keep(func(_ uintptr, args *comObject) uintptr {
		args.call(6, 1) // ICoreWebView2NewWindowRequestedEventArgs.put_Handled
		return 0
	})
	if !w.setup("popup handler failed", w.web.call(44, ptr(popup), ptr(&token))) { // ICoreWebView2.add_NewWindowRequested
		return false
	}
	permission := w.keep(func(_ uintptr, args *comObject) uintptr {
		args.call(7, 2) // ICoreWebView2PermissionRequestedEventArgs.put_State (DENY)
		return 0
	})
	return w.setup("permission handler failed", w.web.call(23, ptr(permission), ptr(&token))) // ICoreWebView2.add_PermissionRequested
}

// blockedNavigation reports whether the login window cancels a navigation to raw: anything
// but plain https to the site, and for a frame also the Cloudflare challenge.
func blockedNavigation(raw string, frame bool) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return true
	}
	return !SiteHost(u.Hostname()) && !(frame && u.Hostname() == "challenges.cloudflare.com")
}
