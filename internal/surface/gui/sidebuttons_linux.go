//go:build cgo

package gui

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The mouse side buttons on the Linux window (16 §3.1 rule 11, §8.5). WebKitGTK hands both
// to the page as `button` 0, so the page cannot tell back from forward, and Wails' own GTK
// handler on the web view keeps the button number to itself. RimArcho connects a second
// `button-press-event` handler to the same web view: Wails' runs first (it records drags),
// then this one, then WebKitGTK's. For buttons 8 and 9 it returns TRUE, so WebKitGTK never
// sees them, and a press sends "history-step" to the page, which takes the step as the
// side buttons do on Windows. GTK is reached through purego, not cgo: RimArcho's packages
// never import "C" (17 §1, D7). The GTK and WebKitGTK symbols are already loaded into the
// process by Wails, so they are looked up with RTLD_DEFAULT.

var gtk struct {
	once sync.Once
	err  error

	signalConnectData    func(instance uintptr, signal string, handler, data, destroy uintptr, flags int32) uint64
	containerGetChildren func(container uintptr) uintptr
	listLength           func(list uintptr) uint32
	listNthData          func(list uintptr, n uint32) uintptr
	listFree             func(list uintptr)
	instanceIsA          func(instance, gtype uintptr) int32
	containerType        func() uintptr
	webViewType          func() uintptr
	eventGetButton       func(event uintptr, button *uint32) int32
	eventGetType         func(event uintptr) int32

	callback uintptr // the one button-press-event handler, made once (purego callbacks are never freed)
	mu       sync.Mutex
	handlers map[uintptr]func(step int) // by the signal's user data
	next     uintptr
}

// loadGTK binds the GTK, GObject and WebKitGTK functions the hook calls.
func loadGTK() error {
	gtk.once.Do(func() {
		bind := func(fn any, name string) {
			if gtk.err != nil {
				return
			}
			sym, err := purego.Dlsym(purego.RTLD_DEFAULT, name)
			if err != nil {
				gtk.err = err
				return
			}
			purego.RegisterFunc(fn, sym)
		}
		bind(&gtk.signalConnectData, "g_signal_connect_data")
		bind(&gtk.containerGetChildren, "gtk_container_get_children")
		bind(&gtk.listLength, "g_list_length")
		bind(&gtk.listNthData, "g_list_nth_data")
		bind(&gtk.listFree, "g_list_free")
		bind(&gtk.instanceIsA, "g_type_check_instance_is_a")
		bind(&gtk.containerType, "gtk_container_get_type")
		bind(&gtk.webViewType, "webkit_web_view_get_type")
		bind(&gtk.eventGetButton, "gdk_event_get_button")
		bind(&gtk.eventGetType, "gdk_event_get_event_type")
		if gtk.err != nil {
			return
		}
		gtk.handlers = map[uintptr]func(int){}
		gtk.callback = purego.NewCallback(onSideButton)
	})
	return gtk.err
}

// onSideButton is the GTK signal handler: gboolean (GtkWidget*, GdkEventButton*, gpointer).
// It runs on GTK's main thread.
func onSideButton(_, event, data uintptr) uintptr {
	var button uint32
	if event == 0 || gtk.eventGetButton(event, &button) == 0 {
		return 0
	}
	step, swallow := sideStep(uint32(gtk.eventGetType(event)), button)
	if step != 0 {
		gtk.mu.Lock()
		h := gtk.handlers[data]
		gtk.mu.Unlock()
		if h != nil {
			h(step)
		}
	}
	if swallow {
		return 1
	}
	return 0
}

// findWebView returns the WebKitWebView inside a GTK widget tree (Wails' window holds a box
// that holds it), or 0. Main thread only.
func findWebView(widget uintptr) uintptr {
	if widget == 0 {
		return 0
	}
	if gtk.instanceIsA(widget, gtk.webViewType()) != 0 {
		return widget
	}
	if gtk.instanceIsA(widget, gtk.containerType()) == 0 {
		return 0
	}
	children := gtk.containerGetChildren(widget)
	defer gtk.listFree(children)
	for i := range gtk.listLength(children) {
		if found := findWebView(gtk.listNthData(children, i)); found != 0 {
			return found
		}
	}
	return 0
}

// hookSideButtons connects the handler to a web view; step gets -1 or 1 on GTK's main thread
// and must not block it. Main thread only.
func hookSideButtons(webView uintptr, step func(int)) error {
	if err := loadGTK(); err != nil {
		return err
	}
	if webView == 0 {
		return errors.New("no web view")
	}
	gtk.mu.Lock()
	gtk.next++
	id := gtk.next
	gtk.handlers[id] = step
	gtk.mu.Unlock()
	if gtk.signalConnectData(webView, "button-press-event", gtk.callback, id, 0, 0) == 0 {
		return errors.New("button-press-event not connected")
	}
	return nil
}

// wireSideButtons hooks the window's web view once its page runtime is ready. Steps reach
// the page in order through one goroutine, so GTK's main thread never waits on the page.
func wireSideButtons(win *application.WebviewWindow, s *API) {
	var once sync.Once
	win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		once.Do(func() {
			steps := make(chan int, 16)
			go func() {
				for step := range steps {
					s.send("history-step", step)
				}
			}()
			application.InvokeAsync(func() {
				err := loadGTK()
				if err == nil {
					err = hookSideButtons(findWebView(uintptr(win.NativeWindow())), func(step int) {
						select {
						case steps <- step:
						default: // the page is far behind; drop the press
						}
					})
				}
				if err != nil {
					application.Get().Logger.Warn("mouse side buttons", "error", err)
				}
			})
		})
	})
}
