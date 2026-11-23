//go:build cgo

package gui

import "testing"

func TestFitSize(t *testing.T) {
	for _, c := range []struct{ aw, ah, w, h int }{
		{1920, 1050, 1440, 880}, // fits
		{1280, 768, 1152, 691},  // the VM's 1280×800 screen less GNOME's top bar
		{700, 400, 720, 480},    // never below the minimum
		{0, 0, 1440, 880},       // unknown screen
	} {
		if w, h := fitSize(1440, 880, c.aw, c.ah); w != c.w || h != c.h {
			t.Errorf("%dx%d: %dx%d, want %dx%d", c.aw, c.ah, w, h, c.w, c.h)
		}
	}
}

func TestVersionBelow(t *testing.T) {
	for v, below := range map[string]bool{"2.42.5": true, "2.44": false, "2.44.0": false, "2.52.6": false, "2.4.11": true, "3.0": false} {
		if versionBelow(v, webkitFloor) != below {
			t.Errorf("%s below %s: %v", v, webkitFloor, !below)
		}
	}
}
