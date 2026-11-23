package app

import (
	"fmt"
	"testing"
)

// The Windows process wrapper's exit error keeps the code: SteamCMD's self-update exit 7
// was reported as a failed download because it did not (VM round 2026-11-10).
func TestProcessExitKeepsCode(t *testing.T) {
	if !steamCMDUpdatedItself(processExit(7)) || !steamCMDUpdatedItself(fmt.Errorf("wait: %w", processExit(7))) {
		t.Fatal("exit 7 not recognised")
	}
	if steamCMDUpdatedItself(processExit(8)) || steamCMDUpdatedItself(fmt.Errorf("process exited with code 7")) {
		t.Fatal("other exits taken for the self-update")
	}
	if got := processExit(7).Error(); got != "process exited with code 7" {
		t.Fatalf("message changed: %q", got)
	}
}
