package upgradewin

import (
	"os"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestInvalidText(t *testing.T) {
	for _, input := range [][2]string{{"bad\x00title", "text"}, {"title", "bad\x00text"}} {
		if w := Open(input[0], input[1]); w != nil {
			w.Close()
			t.Fatal("Open accepted an embedded NUL")
		}
	}
}

func TestNativeLifecycleSmoke(t *testing.T) {
	if os.Getenv("RIMARCHO_UPGRADEWIN_SMOKE") != "1" {
		t.Skip("set RIMARCHO_UPGRADEWIN_SMOKE=1 to test native controls")
	}
	w := Open("RimArcho", "正在升级数据库…")
	if w == nil {
		t.Fatal("native window could not be created")
	}
	t.Cleanup(w.Close)
	hwnd := w.hwnd
	isWindow := user32.NewProc("IsWindow")
	isVisible := user32.NewProc("IsWindowVisible")
	for _, proc := range []*windows.LazyProc{isWindow, isVisible} {
		if err := proc.Find(); err != nil {
			t.Fatal(err)
		}
	}
	if visible, _, _ := isVisible.Call(hwnd); visible == 0 {
		t.Fatal("Open returned before the window was visible")
	}
	assertControls := func(text string, position uintptr) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		var actual string
		var progress uintptr
		for {
			var buffer [256]uint16
			sendMessage.Call(w.label, 0x000D, uintptr(len(buffer)), uintptr(unsafe.Pointer(&buffer[0]))) // WM_GETTEXT
			actual = windows.UTF16ToString(buffer[:])
			progress, _, _ = sendMessage.Call(w.bar, 0x0408, 0, 0) // PBM_GETPOS
			if actual == text && progress == position {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("controls = (%q, %d), want (%q, %d)", actual, progress, text, position)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	assertControls("正在升级数据库…", 0)
	sendMessage.Call(hwnd, 0x0010, 0, 0) // WM_CLOSE, including Alt+F4.
	if alive, _, _ := isWindow.Call(hwnd); alive == 0 {
		t.Fatal("user close destroyed the window")
	}
	w.Update("正在升级索引…", 0.625)
	assertControls("正在升级索引…", 625)
	w.Update("正在收尾…", -1)
	assertControls("正在收尾…", 625)
	w.Update("升级完成", 2)
	assertControls("升级完成", 1000)
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for i := range 200 {
				w.Update("正在升级数据库…", float64(i)/200)
			}
		})
	}
	workers.Wait()
	w.Update("升级完成", 1)
	assertControls("升级完成", 1000)
	for range 8 {
		workers.Go(w.Close)
	}
	workers.Wait()
	select {
	case <-w.done:
	default:
		t.Fatal("Close returned before the window thread finished")
	}
	if alive, _, _ := isWindow.Call(hwnd); alive != 0 {
		t.Fatal("Close left the window alive")
	}
	if _, present := windowsByHandle.Load(hwnd); present {
		t.Fatal("Close left the window registered in Go")
	}
	w.Update("After close", 0.5)
}
