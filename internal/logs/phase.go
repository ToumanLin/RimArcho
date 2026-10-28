package logs

import (
	"fmt"
	"strings"
	"time"
)

// Phase is an observed Monitor boundary (03 §6.7.3), never inferred from an error.
// Index preserves repeated save/play/menu transitions within one run.
type Phase struct {
	Name  string
	At    time.Time `json:",omitempty"`
	Save  string    `json:",omitempty"`
	Index int
	Line  int `json:",omitempty"`
}

type PhaseCount struct {
	Phase *Phase
	Count int
}

var LoadingPhase = &Phase{Name: "loading"}
var UnknownPhase = &Phase{Name: "unknown"}

func ValidPhase(name string) bool {
	switch name {
	case "menu", "save", "new", "play":
		return true
	}
	return false
}

func PhaseName(p *Phase) string {
	if p == nil {
		return "loading"
	}
	return p.Name
}

func AddPhaseCount(cs []PhaseCount, p *Phase, n int) []PhaseCount {
	if p == nil {
		p = LoadingPhase
	}
	for i := range cs {
		if PhaseName(cs[i].Phase) == PhaseName(p) {
			cs[i].Count += n
			return cs
		}
	}
	return append(cs, PhaseCount{Phase: p, Count: n})
}

// MergePhaseCounts takes the larger observed count of each phase, rather than adding
// the same occurrences from Player.log and its companion. Keep that source's boundaries.
func MergePhaseCounts(a, b []PhaseCount) []PhaseCount {
	ta, tb := map[string]int{}, map[string]int{}
	for _, c := range a {
		ta[PhaseName(c.Phase)] += c.Count
	}
	for _, c := range b {
		tb[PhaseName(c.Phase)] += c.Count
	}
	var out []PhaseCount
	for _, c := range a {
		if ta[PhaseName(c.Phase)] > tb[PhaseName(c.Phase)] {
			out = append(out, c)
		}
	}
	for _, c := range b {
		if tb[PhaseName(c.Phase)] >= ta[PhaseName(c.Phase)] {
			out = append(out, c)
		}
	}
	return out
}

// PhaseSummary is the compact CLI/MCP annotation. Empty means the log has no markers.
func PhaseSummary(cs []PhaseCount) string {
	if len(cs) == 0 {
		return ""
	}
	totals := map[string]int{}
	for _, c := range cs {
		totals[PhaseName(c.Phase)] += c.Count
	}
	var parts []string
	for _, name := range []string{"loading", "save", "new", "play", "menu", "unknown"} {
		if n := totals[name]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s x%d", name, n))
		}
	}
	return " [" + strings.Join(parts, ", ") + "]"
}

// CompletePhaseCounts preserves observations whose phase could not be recovered (for
// example Player.log stopped before its first marker and Monitor began logging later).
func CompletePhaseCounts(cs []PhaseCount, total int) []PhaseCount {
	n := 0
	for _, c := range cs {
		n += c.Count
	}
	if n < total {
		cs = AddPhaseCount(cs, UnknownPhase, total-n)
	}
	return cs
}
