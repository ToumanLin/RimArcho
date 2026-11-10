package gametest

import (
	"github.com/ToumanLin/RimArcho/internal/procfs"
	"strconv"
)

func peakMemoryAt(root string, pid int) int64 {
	m, err := procfs.StatusAt(root, strconv.Itoa(pid))
	if err != nil {
		return 0
	}
	return m.Peak / (1 << 20)
}
