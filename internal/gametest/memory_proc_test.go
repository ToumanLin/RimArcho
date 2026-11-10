package gametest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPeakMemoryAt(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "42")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "status")
	if err := os.WriteFile(p, []byte("VmRSS: 2048 kB\nVmHWM: 5120 kB\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := peakMemoryAt(root, 42); got != 5 {
		t.Fatalf("peak = %d MiB", got)
	}
	if got := peakMemoryAt(root, 43); got != 0 {
		t.Fatalf("exited peak = %d", got)
	}
	if err := os.WriteFile(p, []byte("VmHWM: bad kB"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := peakMemoryAt(root, 42); got != 0 {
		t.Fatalf("malformed peak = %d", got)
	}
}
