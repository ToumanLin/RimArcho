package view

import "testing"

// 16 §2.9: a label the game drew with its own colon loses it where a surface adds one.
func TestWindowLabel(t *testing.T) {
	for in, want := range map[string]string{"Search:": "Search", " Name \uff1a": "Name", "Raids": "Raids", "": ""} {
		if got := WindowLabel(in); got != want {
			t.Errorf("WindowLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
