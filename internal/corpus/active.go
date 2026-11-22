package corpus

import (
	"encoding/json"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/known"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/patterns"
)

// EmbeddedData is the data version of the bundle built into the program (A): the week the
// embedded 1.6 files were generated (patterns 2026-10-24, logs through 2026-10).
const EmbeddedData = "2026-W43"

// EmbeddedBundle is A: the program's own files as a bundle. Only 1.6 has a section; other
// game versions get no corpus data until a Monitor bundle has their section (no borrowing).
var EmbeddedBundle = sync.OnceValue(func() *Bundle {
	return &Bundle{Format: Format, Data: EmbeddedData, Sections: map[string]*Section{
		"1.6": {Data: EmbeddedData, Logs: 40326, Window: "2025-W27..2026-W43",
			Patterns: patterns.EmbeddedJSON(), Known: known.EmbeddedJSON(), Observed: order.ObservedJSON()},
	}}
})

// EmbeddedCandidate is A as a candidate.
func EmbeddedCandidate() Candidate {
	return Candidate{Origin: Embedded, Bundle: EmbeddedBundle()}
}

// Active is the data the program uses now: one game version's choice, parsed on first use.
type Active struct {
	Choice Choice

	once     sync.Once
	patterns *patterns.Data
	known    *known.Data
	observed order.ObservedSet
}

func (a *Active) load() {
	a.once.Do(func() {
		s := a.Choice.Section
		a.patterns, a.known, a.observed = emptyPatterns(), &known.Data{}, order.ObservedSet{"harmless": {}, "harmful": {}}
		if s == nil {
			return
		}
		if len(s.Patterns) > 0 {
			if d, err := patterns.Parse(s.Patterns); err == nil {
				a.patterns = d
			}
		}
		if len(s.Known) > 0 {
			if d, err := known.Parse(s.Known); err == nil {
				a.known = d
			}
		}
		if len(s.Observed) > 0 {
			if o, err := order.ParseObserved(s.Observed); err == nil {
				a.observed = o
			}
		}
	})
}

func emptyPatterns() *patterns.Data {
	d, _ := patterns.Parse([]byte(`{"version":"","game":"","logs":0}`))
	return d
}

// There is no process-wide "data in use": each App reads its own environment's choice
// (app.Index), since two Apps of different game versions can share a process (parallel tests,
// MCP switching Game Config per call). Only the parsing is shared: one Active per section.
var (
	mu     sync.Mutex
	parsed = map[string]*Active{}
)

// Activate is ch's data, parsed on first use and shared by every App that chooses the same
// section. It never changes what another App reads.
func Activate(ch Choice) *Active {
	k := ch.Game + "|" + ch.Data + "|" + ch.Origin + "|" + ch.Label
	mu.Lock()
	defer mu.Unlock()
	if a := parsed[k]; a != nil && a.Choice.Section == ch.Section {
		return a
	}
	a := &Active{Choice: ch}
	parsed[k] = a
	return a
}

// Patterns is the pattern library of the choice; nil Active is the embedded one.
func (a *Active) Patterns() *patterns.Data {
	if a == nil {
		return patterns.Embedded()
	}
	a.load()
	return a.patterns
}

// Known is the undeclared prerequisites and known-bad combinations of the choice; nil Active
// is the embedded file. A test's dataset (known.SetForTesting) wins.
func (a *Active) Known() *known.Data {
	if o := known.Override(); o != nil {
		return o
	}
	if a == nil {
		return known.Get()
	}
	a.load()
	return a.known
}

// Observed is the measured load-order constraints of the choice; nil Active is the embedded file.
func (a *Active) Observed() order.ObservedSet {
	if a == nil {
		return order.EmbeddedObserved()
	}
	a.load()
	return a.observed
}

// Summary is the corpus source as interfaces show it (16 §2.4 Can it run, `archo status`,
// MCP status).
type Summary struct {
	Game        string `json:"game"`
	Data        string `json:"data,omitempty"` // ISO week, "" when there is none for this game version
	From        string `json:"from,omitempty"` // embedded | monitor | workshop
	Folder      string `json:"folder,omitempty"`
	Logs        int    `json:"logs,omitempty"`
	NewerFormat bool   `json:"newerFormat,omitempty"`
	Label       string `json:"label"` // "corpus W47, 1.7"
}

// Summarize is the short form of a choice.
func (ch Choice) Summarize() Summary {
	return Summary{Game: ch.Game, Data: ch.Data, From: ch.Origin, Folder: ch.Label, Logs: ch.Logs, NewerFormat: ch.NewerFormat, Label: ch.String()}
}

// MarshalJSON keeps Section out of JSON output.
func (ch Choice) MarshalJSON() ([]byte, error) { return json.Marshal(ch.Summarize()) }
