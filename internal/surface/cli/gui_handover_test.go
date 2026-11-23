package cli

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestGUIProgram(t *testing.T) {
	for goos, want := range map[string]string{"windows": "RimArcho.exe", "linux": "rimarcho", "darwin": ""} {
		if got := guiProgram(goos); got != want {
			t.Errorf("%s: %q, want %q", goos, got, want)
		}
	}
}

// rimarcho started without a desktop says where the text interface is (16 §8.1) instead of
// letting GTK abort.
func TestPreflightNoDisplay(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	old := hasDisplay
	t.Cleanup(func() { hasDisplay = old })
	hasDisplay = func() bool { return false }
	var u *windowUnavailable
	if err := guiPreflight(); !errors.As(err, &u) || !strings.Contains(u.text, "use archo instead") {
		t.Fatalf("got %v", err)
	}
	hasDisplay = func() bool { return true }
	if err := guiPreflight(); err != nil {
		t.Fatal(err)
	}
}
