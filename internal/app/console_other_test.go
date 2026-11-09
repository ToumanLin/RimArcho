//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkshopProcessCancellation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ready, survived := filepath.Join(dir, "ready"), filepath.Join(dir, "survived")
	// A child that outlives cancellation would leave evidence after the parent
	// has been reaped. All paths are synthetic and passed as shell arguments.
	proc, err := startWorkshopCMD("/bin/sh", []string{"-c", `(sleep 1; echo survived > "$2") & echo ready > "$1"; wait`, "sh", ready, survived}, false)
	if err != nil {
		t.Fatal(err)
	}
	killer := proc.(interface{ KillTree() error })
	t.Cleanup(func() { _ = killer.KillTree() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shell did not start its child")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := killer.KillTree(); err != nil {
		t.Fatal(err)
	}
	if err := proc.Wait(); err == nil {
		t.Fatal("killed process reported success")
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := os.Stat(survived); !os.IsNotExist(err) {
		t.Fatalf("descendant survived cancellation: %v", err)
	}
}
