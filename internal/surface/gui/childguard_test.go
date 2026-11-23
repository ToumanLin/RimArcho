//go:build windows || cgo

package gui

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// A helper process RimArcho started never opens a window (17 §3): Run refuses before it
// touches the App, so a nil App is enough.
func TestRunRefusesInChildProcess(t *testing.T) {
	t.Setenv(app.ChildEnvVar, "1")
	err := Run(Options{})
	if err == nil || !strings.Contains(err.Error(), app.ChildEnvVar) {
		t.Fatalf("Run in a child process: %v", err)
	}
}
