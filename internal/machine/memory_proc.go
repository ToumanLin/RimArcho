package machine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ToumanLin/RimArcho/internal/procfs"
)

func readMemoryAt(root string) Memory {
	m := Memory{SampledAt: time.Now(), RAMAvailableUnknown: true}
	f, err := os.Open(filepath.Join(root, "meminfo"))
	if err != nil {
		m.Error = err.Error()
		return m
	}
	defer f.Close()
	parsed, err := parseMeminfo(f)
	if err != nil {
		m.Error = err.Error()
		return m
	}
	parsed.SampledAt = m.SampledAt
	return parsed
}

func parseMeminfo(r io.Reader) (Memory, error) {
	v, err := procfs.ReadKB(r, "MemTotal", "MemAvailable", "MemFree", "Buffers", "Cached", "SReclaimable", "Shmem")
	if err != nil {
		return Memory{}, err
	}
	m := Memory{TotalRAM: v["MemTotal"], RAMAvailableUnknown: true}
	if m.TotalRAM <= 0 {
		return Memory{}, fmt.Errorf("proc meminfo has no positive MemTotal")
	}
	available, ok := v["MemAvailable"]
	if !ok {
		// Older kernels have no MemAvailable. Approximate reclaimable physical
		// memory, removing shared pages already counted in Cached; never add swap.
		if _, ok = v["MemFree"]; ok {
			available = v["MemFree"] + v["Buffers"] + v["Cached"] + v["SReclaimable"] - v["Shmem"]
		}
	}
	if ok {
		m.AvailableRAM = max(0, min(m.TotalRAM, available))
		m.RAMAvailableUnknown = false
	}
	// No Linux dedicated-VRAM or comparable Windows commit counter is probed.
	return m, nil
}
