//go:build cgo

package gui

import (
	"os"
	"runtime"
	"slices"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
)

// TestSideButtonsHookGTK builds Wails' widget tree (a window holding a box holding a
// WebKitWebView) with real GTK 3 and WebKitGTK, hooks it as the window does, and feeds the
// web view button events: 8 and 9 step back and forward and never reach the handlers after
// the hook (WebKitGTK's own comes last); a double press steps once; other buttons pass.
// It needs a display: CI's linux-gui job runs the GUI tests under xvfb-run (17 §5).
func TestSideButtonsHookGTK(t *testing.T) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("no display: the linux-gui job runs these tests under xvfb-run")
		}
		t.Skip("no display")
	}
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("the synthetic GdkEventButton below has 64-bit offsets")
	}
	runtime.LockOSThread() // GTK stays on the thread that initialised it
	defer runtime.UnlockOSThread()
	if err := loadGTK(); err != nil {
		t.Fatal(err)
	}
	var (
		initCheck    func(argc, argv uintptr) int32
		windowNew    func(kind int32) uintptr
		boxNew       func(orientation, spacing int32) uintptr
		labelNew     func(text string) uintptr
		containerAdd func(container, widget uintptr)
		boxPackStart func(box, child uintptr, expand, fill int32, padding uint32)
		webViewNew   func() uintptr
		showAll      func(widget uintptr)
		getWindow    func(widget uintptr) uintptr
		objectRef    func(object uintptr) uintptr
		widgetEvent  func(widget uintptr, event unsafe.Pointer) int32
		eventNew     func(kind int32) unsafe.Pointer
		eventFree    func(event unsafe.Pointer)
		destroy      func(widget uintptr)
	)
	for fn, name := range map[any]string{
		&initCheck: "gtk_init_check", &windowNew: "gtk_window_new", &boxNew: "gtk_box_new",
		&labelNew: "gtk_label_new", &containerAdd: "gtk_container_add", &boxPackStart: "gtk_box_pack_start",
		&webViewNew: "webkit_web_view_new", &showAll: "gtk_widget_show_all", &getWindow: "gtk_widget_get_window",
		&objectRef: "g_object_ref", &widgetEvent: "gtk_widget_event", &eventNew: "gdk_event_new",
		&eventFree: "gdk_event_free", &destroy: "gtk_widget_destroy",
	} {
		sym, err := purego.Dlsym(purego.RTLD_DEFAULT, name)
		if err != nil {
			t.Fatal(err)
		}
		purego.RegisterFunc(fn, sym)
	}
	if initCheck(0, 0) == 0 {
		t.Fatal("gtk_init_check failed")
	}

	window := windowNew(0) // GTK_WINDOW_TOPLEVEL
	box := boxNew(1, 0)    // GTK_ORIENTATION_VERTICAL, as Wails' windowNew
	label := labelNew("menu")
	webView := webViewNew()
	containerAdd(window, box)
	boxPackStart(box, label, 0, 0, 0)
	boxPackStart(box, webView, 1, 1, 0)
	showAll(window)
	defer destroy(window)

	if got := findWebView(window); got != webView {
		t.Fatalf("findWebView(window) = %#x, want the web view %#x", got, webView)
	}
	if got := findWebView(label); got != 0 {
		t.Fatalf("findWebView(label) = %#x, want 0", got)
	}

	var steps []int
	if err := hookSideButtons(webView, func(s int) { steps = append(steps, s) }); err != nil {
		t.Fatal(err)
	}
	// Connected after the hook, as WebKitGTK's class handler runs after it. It returns TRUE
	// so the synthetic events never reach WebKitGTK itself.
	var passed []uint32
	probe := purego.NewCallback(func(_, event, _ uintptr) uintptr {
		var b uint32
		gtk.eventGetButton(event, &b)
		passed = append(passed, b)
		return 1
	})
	if gtk.signalConnectData(webView, "button-press-event", probe, 0, 0, 0) == 0 {
		t.Fatal("probe not connected")
	}

	gdkWindow := getWindow(webView)
	if gdkWindow == 0 {
		t.Fatal("the web view has no GdkWindow (not realized)")
	}
	send := func(kind int32, button uint32) {
		ev := eventNew(kind)
		// GdkEventButton on 64-bit: window at 8, button at 52 (GTK 3 has no setter for it).
		*(*uintptr)(unsafe.Add(ev, 8)) = objectRef(gdkWindow) // gdk_event_free unrefs it
		*(*uint32)(unsafe.Add(ev, 52)) = button
		widgetEvent(webView, ev)
		eventFree(ev)
	}
	send(4, 8) // GDK_BUTTON_PRESS, back
	send(4, 9) // forward
	send(4, 8) // a quick second back press: GDK_BUTTON_PRESS, then GDK_2BUTTON_PRESS
	send(5, 8)
	send(4, 1) // the left button
	send(5, 1)
	if want := []int{-1, 1, -1}; !slices.Equal(steps, want) {
		t.Errorf("steps = %v, want %v", steps, want)
	}
	if want := []uint32{1, 1}; !slices.Equal(passed, want) {
		t.Errorf("events past the hook carried buttons %v, want %v (8 and 9 must be swallowed)", passed, want)
	}
}
