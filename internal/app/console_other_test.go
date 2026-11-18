//go:build !windows

package app

import (
	"io"
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
	// The program sits in the temporary folder, so SteamCMD's own home goes there too.
	sh := filepath.Join(dir, "sh")
	if err := os.WriteFile(sh, []byte("#!/bin/sh\nexec /bin/sh \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	proc, err := startWorkshopCMD(sh, []string{"-c", `(sleep 1; echo survived > "$2") & echo ready > "$1"; wait`, "sh", ready, survived}, false)
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

// Under `archo mcp`, stdin and stdout are the protocol's pipes (14 §1): an interactive
// SteamCMD run must neither write into stdout nor read the client's requests from stdin.
// Not parallel: it swaps os.Stdin and os.Stdout while parallel tests are still paused.
func TestWorkshopConsoleNeedsTerminal(t *testing.T) {
	dir := t.TempDir()
	sh := filepath.Join(dir, "sh")
	if err := os.WriteFile(sh, []byte("#!/bin/sh\nexec /bin/sh \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inW.WriteString("request\n"); err != nil {
		t.Fatal(err)
	}
	inW.Close()
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inR, outW
	proc, err := startWorkshopCMD(sh, []string{"-c", `read x; echo "leaked $x"`}, true)
	os.Stdin, os.Stdout = oldIn, oldOut
	if err != nil {
		t.Fatal(err)
	}
	_ = proc.Wait()
	outW.Close()
	out, _ := io.ReadAll(outR)
	in, _ := io.ReadAll(inR)
	if len(out) != 0 || string(in) != "request\n" {
		t.Fatalf("SteamCMD inherited the protocol pipes: stdout got %q, stdin left %q", out, in)
	}
}
