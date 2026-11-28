package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/oplog"
)

// CheckPhases is what a can-it-run check's row records (oplog.Check, 13 §1.3): the game
// version, how many Mods it read, whether it was automatic, and how long each phase took.
// Its detail is one line that fits the row's 400 characters:
//
//	1.6, 312/1354 Mods: read 320 ms, defs 1.2 s, patches 3.4 s, harmony 800 ms, save 500 ms, refs 300 ms
//
// with ", automatic" after the counts for an automatic check (16 §8.9). The slow-check
// report (16 §2.14) reads it back with ParseCheckPhases.
type CheckPhases struct {
	Game          string
	Active, Total int
	Automatic     bool
	Phases        []CheckPhase
}

// CheckPhase is one phase and its milliseconds.
type CheckPhase struct {
	Name string `json:"name"`
	Ms   int64  `json:"ms"`
}

// Add appends a phase.
func (p *CheckPhases) Add(name string, ms int64) {
	p.Phases = append(p.Phases, CheckPhase{Name: name, Ms: max(0, ms)})
}

// Detail is the row's detail line.
func (p CheckPhases) Detail() string {
	if p.Total == 0 && len(p.Phases) == 0 {
		return ""
	}
	var b strings.Builder
	if p.Game != "" {
		b.WriteString(p.Game + ", ")
	}
	fmt.Fprintf(&b, "%d/%d Mods", p.Active, p.Total)
	if p.Automatic {
		b.WriteString(", automatic")
	}
	for i, ph := range p.Phases {
		if i == 0 {
			b.WriteString(": ")
		} else {
			b.WriteString(", ")
		}
		b.WriteString(ph.Name + " " + oplog.Dur(ph.Ms))
	}
	return b.String()
}

var (
	checkHeadRe  = regexp.MustCompile(`^(?:([0-9.]+), )?(\d+)/(\d+) Mods(, automatic)?(?:: (.*))?$`)
	checkPhaseRe = regexp.MustCompile(`^([a-z]+) (\d+(?:\.\d)?) (ms|s|min)(?: (\d+) s)?$`)
)

// ParseCheckPhases reads a check row's detail back; ok is false when it is not one (a
// failed check's error follows the phases after "; ", and is ignored).
func ParseCheckPhases(detail string) (CheckPhases, bool) {
	line, _, _ := strings.Cut(detail, "; ")
	m := checkHeadRe.FindStringSubmatch(line)
	if m == nil {
		return CheckPhases{}, false
	}
	p := CheckPhases{Game: m[1], Automatic: m[4] != ""}
	p.Active, _ = strconv.Atoi(m[2])
	p.Total, _ = strconv.Atoi(m[3])
	if m[5] == "" {
		return p, true
	}
	for _, part := range strings.Split(m[5], ", ") {
		pm := checkPhaseRe.FindStringSubmatch(part)
		if pm == nil {
			return p, false
		}
		v, _ := strconv.ParseFloat(pm[2], 64)
		var ms int64
		switch pm[3] {
		case "ms":
			ms = int64(v)
		case "s":
			ms = int64(v*1000 + 0.5)
		case "min":
			s, _ := strconv.Atoi(pm[4])
			ms = int64(v)*60_000 + int64(s)*1000
		}
		p.Add(pm[1], ms)
	}
	return p, true
}

// Ms is how long the phase named name took (0 when there is none).
func (p CheckPhases) Ms(name string) int64 {
	for _, ph := range p.Phases {
		if ph.Name == name {
			return ph.Ms
		}
	}
	return 0
}
