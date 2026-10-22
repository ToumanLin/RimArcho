// Package known holds what the log corpus observed about mods: prerequisites their authors
// did not declare, and combinations that produce the same error in many shared logs
// (docs/decisions-corpus.md §2, generator spikes/knowndeps). Aggregate counts only: no log
// ids, no full mod lists; every entry was seen in the logs of at least five different players.
//
// The data is embedded: `archo data update` only fetches third-party datasets, and RimArcho
// has no data repository of its own yet (the same choice as internal/order/observed.json).
package known

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Prereq: when Mod is enabled and Needs is not, an error names Needs as the provider of
// something Mod uses (a type, a parent def, a def, a texture). Mod's About.xml does not
// declare Needs (declared dependencies are dep.inactive / dep.missing).
type Prereq struct {
	Mod       string   `json:"mod"`
	Needs     string   `json:"needs"`
	Logs      int      `json:"logs"`                // logs with Mod enabled, Needs not, and such an error
	Players   int      `json:"players"`             // … from this many different players (mod lists)
	Without   int      `json:"without"`             // logs with Mod enabled and Needs not
	Items     []string `json:"items"`               // what was missing, e.g. "def SoundDef X", "type Ns.T"
	Class     string   `json:"class"`               // log class of the most common error
	Example   string   `json:"example"`             // its template (names and numbers kept, paths dropped)
	Cosmetic  bool     `json:"cosmetic,omitempty"`  // only sounds or textures are missing
	LoadAfter bool     `json:"loadAfter,omitempty"` // Mod lists Needs in loadAfter (optional on purpose)
	Fix       *Fix     `json:"fix,omitempty"`
}

// Combo: an error attributed to Mods[0] alone (RimArcho named it the source). Three kinds:
//   - pair: the corpus sees it mostly when Mods[1] is enabled too, rarely without;
//   - version: mostly with Mods[0] at Version, rarely at its other versions;
//   - mod: in at least half of the shared logs that enable Mods[0] (a known error of the mod).
type Combo struct {
	Mods       []string `json:"mods"`
	Version    string   `json:"version,omitempty"` // Mods[0]'s version as HugsLib logs it
	Class      string   `json:"class"`
	Templates  []string `json:"templates"`  // most common first
	Logs       int      `json:"logs"`       // logs with the combination and the error
	Players    int      `json:"players"`    // … from this many different players
	With       int      `json:"with"`       // logs with the combination
	Without    int      `json:"without"`    // logs with Mods[0] but not the combination (0 for kind mod)
	WithoutErr int      `json:"withoutErr"` // … that still have the error
	Fix        *Fix     `json:"fix,omitempty"`
	G5         *G5      `json:"g5,omitempty"`
}

// Combination kinds.
const (
	KindPair    = "pair"
	KindVersion = "version"
	KindMod     = "mod"
)

// Kind is pair, version or mod.
func (c *Combo) Kind() string {
	switch {
	case len(c.Mods) > 1:
		return KindPair
	case c.Version != "":
		return KindVersion
	}
	return KindMod
}

// Fix is the session evidence (consecutive uploads of one player, design §5 L1): of the
// uploads that followed one with the error, how many no longer had it, and what changed
// about the entry's mods then. Co-occurrence only, not proof that the change fixed it.
type Fix struct {
	Pairs    int            `json:"pairs"`
	Gone     int            `json:"gone"`
	Same     int            `json:"same"`     // pairs where these mods did not change (the control)
	SameGone int            `json:"sameGone"` // … and the error was gone anyway
	Changes  map[string]int `json:"changes,omitempty"`
	Source   string         `json:"source"`
}

// G5 is the resolution evidence of the G5 pattern library (spikes/patterns) for the entry's
// most observed template: consecutive uploads with no list change and with 1–3 changes, how
// many lost the error, and what changed in the latter.
type G5 struct {
	SamePairs  int            `json:"samePairs"`
	SameGone   int            `json:"sameGone"`
	SmallPairs int            `json:"smallPairs"`
	SmallGone  int            `json:"smallGone"`
	Fixes      map[string]int `json:"fixes,omitempty"`
}

// Text is a one-line summary.
func (g *G5) Text() string {
	s := fmt.Sprintf("gone after a small list change %d/%d, without a change %d/%d", g.SmallGone, g.SmallPairs, g.SameGone, g.SamePairs)
	if len(g.Fixes) > 0 {
		var ks []string
		for k := range g.Fixes {
			ks = append(ks, k)
		}
		sort.Slice(ks, func(i, j int) bool {
			return g.Fixes[ks[i]] > g.Fixes[ks[j]] || g.Fixes[ks[i]] == g.Fixes[ks[j]] && ks[i] < ks[j]
		})
		var parts []string
		for _, k := range ks {
			parts = append(parts, fmt.Sprintf("%s %d", k, g.Fixes[k]))
		}
		s += " (" + strings.Join(parts, ", ") + ")"
	}
	return s
}

// Data is the embedded dataset.
type Data struct {
	Source  string    `json:"source"`
	Game    string    `json:"game"`
	Logs    int       `json:"logs"`
	Prereqs []*Prereq `json:"prereqs"`
	Combos  []*Combo  `json:"combos"`

	byMod   map[string][]*Prereq
	byCombo map[string][]*Combo
}

//go:embed known.json
var knownJSON []byte

var embedded = sync.OnceValue(func() *Data {
	d, err := Parse(knownJSON)
	if err != nil {
		return &Data{}
	}
	return d
})

var override *Data

// Get returns the dataset in use.
func Get() *Data {
	if override != nil {
		return override
	}
	return embedded()
}

// SetForTesting replaces the dataset until the returned function is called.
func SetForTesting(d *Data) (restore func()) {
	old := override
	d.index()
	override = d
	return func() { override = old }
}

// Parse reads a dataset.
func Parse(b []byte) (*Data, error) {
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	d.index()
	return &d, nil
}

func norm(p string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(p)), "_steam")
}

func (d *Data) index() {
	d.byMod = map[string][]*Prereq{}
	d.byCombo = map[string][]*Combo{}
	for _, p := range d.Prereqs {
		p.Mod, p.Needs = norm(p.Mod), norm(p.Needs)
		d.byMod[p.Mod] = append(d.byMod[p.Mod], p)
	}
	for _, c := range d.Combos {
		for i := range c.Mods {
			c.Mods[i] = norm(c.Mods[i])
		}
		if len(c.Mods) > 0 {
			d.byCombo[c.Mods[0]] = append(d.byCombo[c.Mods[0]], c)
		}
	}
}

// PrereqsOf returns the observed prerequisites of a package.
func (d *Data) PrereqsOf(pkg string) []*Prereq { return d.byMod[norm(pkg)] }

// CombosOf returns the combinations whose first mod is pkg.
func (d *Data) CombosOf(pkg string) []*Combo { return d.byCombo[norm(pkg)] }

// Rate is the share of logs with Mod enabled and Needs not that had the error.
func (p *Prereq) Rate() float64 { return ratio(p.Logs, p.Without) }

// Rate is the share of logs with the combination that had the error.
func (c *Combo) Rate() float64 { return ratio(c.Logs, c.With) }

// RateWithout is the share of logs with Mods[0] but not the combination that had it.
func (c *Combo) RateWithout() float64 { return ratio(c.WithoutErr, c.Without) }

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
