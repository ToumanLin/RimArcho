package subscribe

import (
	"encoding/json"
	"os"
	"testing"
)

// The same table runs against the window's detect (surface/gui/testdata/subscribe.mjs), so
// the TUI and MCP recognise what the GUI's paste box recognises.
func TestDetect(t *testing.T) {
	b, err := os.ReadFile("testdata/detect.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases map[string]string
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for text, want := range cases {
		if got := Detect(text); got != want {
			t.Errorf("Detect(%q) = %q, want %q", text, got, want)
		}
	}
}
