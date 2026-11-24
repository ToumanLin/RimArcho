//go:build cgo

package gui

import (
	"slices"
	"testing"
)

// TestOnSignalStack: a handler without SA_ONSTACK is listed and moved to the signal stack;
// SIGUSR1 (JavaScriptCore's) is never touched. It uses a real-time signal no one sends, whose
// handler is Go's own, so the process is never at risk.
func TestOnSignalStack(t *testing.T) {
	const sig = 40
	var orig kernelSigaction
	if err := sigaction(sig, nil, &orig); err != nil {
		t.Fatal(err)
	}
	if orig.handler <= 1 || orig.flags&saOnStack == 0 {
		t.Fatalf("signal %d: Go's handler expected, got %#x flags %#x", sig, orig.handler, orig.flags)
	}
	t.Cleanup(func() { sigaction(sig, &orig, nil) })
	off := orig
	off.flags &^= saOnStack
	if err := sigaction(sig, &off, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(offStack(), sig) {
		t.Fatalf("offStack() = %v, want %d in it", offStack(), sig)
	}
	if fixed := onSignalStack(); !slices.Contains(fixed, sig) {
		t.Fatalf("onSignalStack() = %v, want %d in it", fixed, sig)
	}
	var now kernelSigaction
	if err := sigaction(sig, nil, &now); err != nil {
		t.Fatal(err)
	}
	if now.flags != orig.flags || now.handler != orig.handler || now.mask != orig.mask || now.restorer != orig.restorer {
		t.Errorf("after the repair: %+v, want %+v", now, orig)
	}
	if slices.Contains(offStack(), sig) {
		t.Error("still listed after the repair")
	}
}
