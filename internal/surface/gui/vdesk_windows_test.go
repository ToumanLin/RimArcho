//go:build windows

package gui

import "testing"

// Only WebView2's window-sized popup in RimArcho's own browser process lets clicks through
// (16 §8.2): Chromium's real popups and other apps' windows keep their input.
func TestWebviewPopupOnlyOwnWindowSizedPopup(t *testing.T) {
	main := rect{560, 256, 2000, 1136}
	children := map[uint32]bool{10: true, 20: false}
	const observed = 0x08280080 // the popup's ex-style as seen on Windows 11 (issue #29)
	cases := []struct {
		name string
		w    popupWindow
		want bool
	}{
		{"the popup", popupWindow{pid: 10, ex: observed, r: main}, true},
		{"select list", popupWindow{pid: 10, ex: observed, r: rect{600, 300, 800, 500}}, false},
		{"not layered", popupWindow{pid: 10, ex: wsExToolWindow, r: main}, false},
		{"another app", popupWindow{pid: 20, ex: observed, r: main}, false},
		{"unknown process", popupWindow{pid: 30, ex: observed, r: main}, false},
	}
	for _, c := range cases {
		if got := webviewPopup(c.w, main, children); got != c.want {
			t.Errorf("%s: webviewPopup = %v, want %v", c.name, got, c.want)
		}
	}
}

// With no window handle nothing is touched.
func TestPassThroughWebviewPopupNeedsWindow(t *testing.T) {
	if n := passThroughWebviewPopup(0); n != 0 {
		t.Fatalf("passThroughWebviewPopup(0) = %d", n)
	}
}
