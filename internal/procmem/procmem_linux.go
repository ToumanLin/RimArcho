package procmem

import "github.com/ToumanLin/RimArcho/internal/procfs"

// MB returns RSS, anonymous RSS (not committed private bytes), and peak RSS in MiB.
func MB() (ws, private, peak float64) {
	m, err := procfs.StatusAt("/proc", "self")
	if err != nil {
		return 0, 0, 0
	}
	return float64(m.RSS) / (1 << 20), float64(m.Private) / (1 << 20), float64(m.Peak) / (1 << 20)
}
