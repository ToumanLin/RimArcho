package order

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Strength says how much breaking a constraint matters (04 §4.4). It comes from the log
// corpus (docs/decisions-corpus.md §2): for every constraint that some shared log breaks,
// how often either mod is an error source when the constraint is broken, against logs with
// both mods that keep it (same pair, same list-size band). Kinds at 1.3 or more are strong.
//
//	kind                                     broken  observed/expected sources
//	dependency between mods                   2,903   1.41
//	loadBefore between mods                     395   1.34
//	loadAfter between mods                    5,430   1.14
//	community rule between mods               6,541   1.13
//	a mod that needs Core/a DLC loads first     192   3–4.5 (dependency, loadAfter, forceLoadAfter)
//	a mod declares it loads before Core/DLC     451   1.05 (superniquito.modoptionssort: 1 of 651)
type Strength int

const (
	// Harmless: breaking it showed no more errors than keeping it.
	Harmless Strength = iota
	// Weak: declared or community ordering preferences; a small effect.
	Weak
	// Strong: dependencies, loadBefore, forced orders, user rules, what needs official content.
	Strong
)

func (s Strength) String() string {
	switch s {
	case Harmless:
		return "harmless"
	case Weak:
		return "weak"
	}
	return "strong"
}

// Assessment is the strength of one constraint and why.
type Assessment struct {
	Strength Strength
	Why      string // one line, empty for the plain kind rule
}

// Observed is one rule's corpus record.
type Observed struct {
	Kind        string `json:"kind"`
	From        string `json:"from"`
	To          string `json:"to"`
	Violated    int    `json:"violated"`    // logs that break it
	ViolatedSrc int    `json:"violatedSrc"` // … where a problem names either mod as a source
	Kept        int    `json:"kept"`        // logs with both mods that keep it
	KeptSrc     int    `json:"keptSrc"`
}

// observed.json: rules the corpus saw broken often enough to judge on their own. Harmless:
// broken in ≥ 30 logs, errors from either mod no more frequent than when kept (or ≤ 2% in
// ≥ 100 logs when too few keep it). Harmful: broken in ≥ 20 logs, errors from either mod
// in ≥ 20% of them, at least twice the rate when kept, z ≥ 3.
//
//go:embed observed.json
var observedJSON []byte

var observed = sync.OnceValue(func() map[string]map[string]*Observed {
	var raw struct {
		Harmless []*Observed `json:"harmless"`
		Harmful  []*Observed `json:"harmful"`
	}
	out := map[string]map[string]*Observed{"harmless": {}, "harmful": {}}
	if json.Unmarshal(observedJSON, &raw) != nil {
		return out
	}
	for _, o := range raw.Harmless {
		out["harmless"][obsKey(o.Kind, o.From, o.To)] = o
	}
	for _, o := range raw.Harmful {
		out["harmful"][obsKey(o.Kind, o.From, o.To)] = o
	}
	return out
})

func obsKey(kind, from, to string) string {
	return kind + " " + strings.ToLower(from) + " " + strings.ToLower(to)
}

// LookupObserved returns the corpus record of a constraint and whether it was harmful
// (nil when the corpus has none).
func LookupObserved(kind, from, to string) (o *Observed, harmful bool) {
	k := obsKey(kind, from, to)
	if o := observed()["harmful"][k]; o != nil {
		return o, true
	}
	return observed()["harmless"][k], false
}

func (o *Observed) text() string {
	return fmt.Sprintf("log corpus: broken in %d shared logs, either mod an error source in %d (%.0f%%); kept in %d, %d (%.0f%%)",
		o.Violated, o.ViolatedSrc, pct(o.ViolatedSrc, o.Violated), o.Kept, o.KeptSrc, pct(o.KeptSrc, o.Kept))
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

// Assess rates one constraint edge of in.
func Assess(in Input, e Edge) Assessment {
	from, to := in.Nodes[e.From].PackageID, in.Nodes[e.To].PackageID
	switch e.C.Kind {
	case KindUser:
		return Assessment{Strong, ""}
	case KindOfficial:
		return Assessment{Strong, ""}
	}
	if o, harmful := LookupObserved(e.C.Kind, from, to); o != nil {
		if harmful {
			return Assessment{Strong, o.text()}
		}
		return Assessment{Harmless, o.text()}
	}
	fo, to2 := officialRank(from) >= 0, officialRank(to) >= 0
	switch {
	case !fo && to2 && beforeOfficial(in, e):
		return Assessment{Harmless, "loading before Core or a DLC is a preference: breaking it showed no more errors in the log corpus"}
	case fo && !to2:
		switch e.C.Kind {
		case KindDependency, KindLoadAfter, KindForceAft:
			return Assessment{Strong, ""}
		}
		return Assessment{Weak, ""}
	}
	switch e.C.Kind {
	case KindDependency, KindLoadBefore, KindForceAft, KindForceBef:
		return Assessment{Strong, ""}
	}
	return Assessment{Weak, ""} // loadAfter, community
}
