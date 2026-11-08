package app

import "strings"

// evidenceRank orders candidates of the same role (LogCluster.order, 06 §3): a Mod the null
// trace shows passing the null, then file or patch evidence, a message prefix, the topmost Mod
// frame, other frames and Harmony lines, anything else. Lower is stronger.
func evidenceRank(at *Attribution) int {
	best := 9
	for _, ev := range at.Evidence {
		r := 8
		switch {
		case strings.Contains(ev, "passes null for the argument"):
			r = 0
		case strings.HasPrefix(ev, "log file "), strings.HasPrefix(ev, "patch "):
			r = 1
		case strings.HasPrefix(ev, "log message prefix "):
			r = 2
		case strings.Contains(ev, "(topmost mod code in the stack)"):
			r = 3
		case strings.HasPrefix(ev, "log frame "), strings.HasPrefix(ev, "log harmony:"):
			r = 4
		}
		best = min(best, r)
	}
	return best
}
