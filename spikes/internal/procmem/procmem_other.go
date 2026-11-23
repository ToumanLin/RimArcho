//go:build !windows

package procmem

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// MB returns (resident set, anonymous resident memory, peak resident set) in MiB, from
// /proc/self/status: VmRSS, RssAnon and VmHWM, the nearest Linux measures to Windows' working
// set, private bytes and peak working set. Without /proc (macOS) all three are 0.
func MB() (ws, private, peak float64) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		kb, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		switch key {
		case "VmRSS":
			ws = kb / 1024
		case "RssAnon":
			private = kb / 1024
		case "VmHWM":
			peak = kb / 1024
		}
	}
	return ws, private, peak
}
