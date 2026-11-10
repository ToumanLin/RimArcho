package machine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMeminfo(t *testing.T) {
	for _, tc := range []struct {
		name, text       string
		total, available int64
		unknown, bad     bool
	}{
		{name: "modern", text: "MemTotal: 8192 kB\nMemAvailable: 3072 kB\nMemFree: 100 kB\nSwapFree: 9999 kB", total: 8192 * 1024, available: 3072 * 1024},
		{name: "legacy", text: "MemTotal: 8192 kB\nMemFree: 1000 kB\nBuffers: 200 kB\nCached: 500 kB\nSReclaimable: 100 kB\nShmem: 300 kB", total: 8192 * 1024, available: 1500 * 1024},
		{name: "zero", text: "MemTotal: 8192 kB\nMemAvailable: 0 kB", total: 8192 * 1024},
		{name: "partial", text: "MemTotal: 8192 kB", total: 8192 * 1024, unknown: true},
		{name: "clamp high", text: "MemTotal: 10 kB\nMemAvailable: 20 kB", total: 10 * 1024, available: 10 * 1024},
		{name: "clamp low", text: "MemTotal: 10 kB\nMemFree: 2 kB\nShmem: 4 kB", total: 10 * 1024},
		{name: "empty", bad: true},
		{name: "zero total", text: "MemTotal: 0 kB", bad: true},
		{name: "invalid", text: "MemTotal: 8192 kB\nMemAvailable: -1 kB", bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseMeminfo(strings.NewReader(tc.text))
			if tc.bad {
				if err == nil {
					t.Fatal("accepted invalid meminfo")
				}
				return
			}
			if err != nil || m.TotalRAM != tc.total || m.AvailableRAM != tc.available || m.RAMAvailableUnknown != tc.unknown || m.VRAM() != 0 || m.CommitAvailable != 0 {
				t.Fatalf("%+v: %v", m, err)
			}
		})
	}
}

func TestReadMemoryAt(t *testing.T) {
	root := t.TempDir()
	m := readMemoryAt(root)
	if m.Error == "" || m.SampledAt.IsZero() || !m.RAMAvailableUnknown {
		t.Fatalf("missing meminfo: %+v", m)
	}
	p := filepath.Join(root, "meminfo")
	if err := os.WriteFile(p, []byte("MemTotal: 4096 kB\nMemAvailable: 2048 kB\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m = readMemoryAt(root)
	if m.Error != "" || m.SampledAt.IsZero() || m.TotalRAM != 4<<20 || m.AvailableRAM != 2<<20 {
		t.Fatalf("%+v", m)
	}
}
