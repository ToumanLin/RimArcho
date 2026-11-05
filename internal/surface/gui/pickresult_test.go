package gui

import (
	"errors"
	"testing"
)

// Cancelling a native picker is an empty choice, not an error (VM round 2026-11-05: the
// status line showed "✗ cancelled by user"). Other errors pass through.
func TestPickResultCancel(t *testing.T) {
	t.Parallel()
	if p, err := pickResult("", errors.New("cancelled by user")); p != "" || err != nil {
		t.Fatalf("cancel: %q %v", p, err)
	}
	if p, err := pickResult(`C:\x`, nil); p != `C:\x` || err != nil {
		t.Fatalf("pick: %q %v", p, err)
	}
	if _, err := pickResult("", errors.New("access denied")); err == nil {
		t.Fatal("a real error was swallowed")
	}
}
