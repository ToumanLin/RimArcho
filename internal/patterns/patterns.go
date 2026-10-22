// Package patterns is the error pattern library (docs/research/00-日志语料顶层设计.md §3 G5,
// §9): kinds of Player.log problems seen in public HugsLib logs, with a recognition rule over
// the cluster template (internal/logs), how often they occur, their usual causes, the
// diagnosis route (which tools to call next) and the fix types.
//
// A pattern is community background (evidence source "community"), never a conclusion about
// the player's list on its own: it says what this kind of error usually is, not which mod
// causes it here.
//
// Two layers:
//   - generic patterns (g:<id>): hand-written rules, first match wins; "fallback" rules only
//     name the class (bare NullReferenceException etc.) and are not shown as a match;
//   - mod-specific patterns (m:<packageId>:<hash>): one template that public logs attributed
//     to one source mod, shown only when the cluster's own attribution names that mod.
//
// The data (patterns.json) is generated from the research dataset by
// `go run ./spikes/patterns -export internal/patterns/patterns.json`; the same command with
// -verify checks that this matcher classifies the corpus as the research did.
package patterns

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Pattern is one entry of the library.
type Pattern struct {
	ID string `json:"id"` // g:<rule> or m:<packageId>:<hash>

	// Generic layer: the recognition rule and what an agent should do with it.
	Class    string   `json:"class,omitempty"` // the cluster class it applies to ("" any)
	Match    string   `json:"match,omitempty"` // regular expression over the template's message part
	Fallback bool     `json:"fallback,omitempty"`
	Stage    string   `json:"stage,omitempty"` // load | save | runtime | noise
	Summary  string   `json:"summary,omitempty"`
	Causes   []string `json:"causes,omitempty"`
	Route    []string `json:"route,omitempty"` // MCP calls, in order
	Fixes    []string `json:"fixes,omitempty"` // disable | enable-dependency | reorder | update | local-fix | ignore | new-game
	// Workflow: whether the MCP diagnosis workflow covered this kind before the library (yes |
	// partial | no; a judgement of the research, kept for the record).
	Workflow string `json:"workflow,omitempty"`

	// Mod layer: the exact cluster template, its source mod and the generic kind.
	Template string   `json:"template,omitempty"`
	Mod      string   `json:"mod,omitempty"`
	Kind     string   `json:"kind,omitempty"` // g:<rule> of the template
	Versions []string `json:"versions,omitempty"`
	First    string   `json:"first,omitempty"` // month first / last seen
	Last     string   `json:"last,omitempty"`

	Logs      int         `json:"logs"`
	Sessions  int         `json:"sessions"`
	NClusters int         `json:"clusters"`
	Res       *Resolution `json:"res,omitempty"`

	re *regexp.Regexp
}

// Resolution is the session evidence: consecutive uploads of one player where the first has
// the pattern (Pairs, by list change between them: none, order, small = 1-3 package or version
// changes) and the second no longer has it (Gone). Fixes: what changed in the small pairs
// where it went (disable-source, disable-other, enable, update-other, reorder). It co-occurs
// with the change; it is not causal.
type Resolution struct {
	Pairs map[string]int `json:"pairs"`
	Gone  map[string]int `json:"gone,omitempty"`
	Fixes map[string]int `json:"fixes,omitempty"`
}

// Data is the whole library.
type Data struct {
	Version   string     `json:"version"`
	Game      string     `json:"game"`
	Logs      int        `json:"logs"`     // public logs the counts come from
	Clusters  int        `json:"clusters"` // problem clusters in them
	Generated string     `json:"generated"`
	Rules     []*Pattern `json:"generic,omitempty"` // generic patterns, in match order
	Mod       []*Pattern `json:"mod,omitempty"`

	byID  map[string]*Pattern
	byTpl map[string][]*Pattern
}

//go:embed patterns.json
var dataJSON []byte

// Load returns the embedded library (parsed once).
var Load = sync.OnceValue(func() *Data {
	d, err := Parse(dataJSON)
	if err != nil {
		panic("internal/patterns: " + err.Error())
	}
	return d
})

// Parse reads a library file and compiles its rules.
func Parse(b []byte) (*Data, error) {
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	d.byID = map[string]*Pattern{}
	d.byTpl = map[string][]*Pattern{}
	for _, p := range d.Rules {
		re, err := regexp.Compile(p.Match)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.ID, err)
		}
		p.re = re
		d.byID[p.ID] = p
	}
	for _, p := range d.Mod {
		d.byID[p.ID] = p
		d.byTpl[p.Template] = append(d.byTpl[p.Template], p)
	}
	return &d, nil
}

// Generic returns the first generic pattern whose rule matches a cluster (class and template
// "class|message" as internal/logs builds them), fallback rules included; nil if none.
func (d *Data) Generic(class, template string) *Pattern {
	msg := template
	if i := strings.IndexByte(template, '|'); i >= 0 {
		msg = template[i+1:]
	}
	for _, p := range d.Rules {
		if p.Class != "" && p.Class != class {
			continue
		}
		if p.re.MatchString(msg) {
			return p
		}
	}
	return nil
}

// ForMod returns the mod-specific pattern of a template whose source mod is one of pkgs
// (lower-case packageIds, without the _steam suffix).
func (d *Data) ForMod(template string, pkgs []string) *Pattern {
	for _, p := range d.byTpl[template] {
		for _, k := range pkgs {
			if p.Mod == k {
				return p
			}
		}
	}
	return nil
}

// Match is what the library says about one cluster.
type Match struct {
	Generic *Pattern // nil when only a fallback rule (or none) matches
	Mod     *Pattern // the mod-specific pattern of one of the cluster's source mods
}

// MatchCluster matches one cluster: its class, template and the packageIds of the mods the
// cluster's own attribution names as source.
func (d *Data) MatchCluster(class, template string, sources []string) Match {
	var m Match
	if g := d.Generic(class, template); g != nil && !g.Fallback {
		m.Generic = g
	}
	if len(sources) > 0 {
		m.Mod = d.ForMod(template, sources)
	}
	return m
}

// Get returns a pattern by id: g:<rule>, m:<pkg>:<hash>, with or without the pattern: prefix;
// a bare rule id (xml.type-missing) means the generic pattern.
func (d *Data) Get(id string) *Pattern {
	id = strings.TrimPrefix(strings.TrimSpace(id), "pattern:")
	if p := d.byID[id]; p != nil {
		return p
	}
	return d.byID["g:"+id]
}

// OfMod lists the mod-specific patterns of one packageId, most frequent first.
func (d *Data) OfMod(pkg string) []*Pattern {
	var out []*Pattern
	for _, p := range d.Mod {
		if p.Mod == pkg {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Logs > out[j].Logs })
	return out
}

// Ref is the citable reference (pattern:<id>).
func (p *Pattern) Ref() string { return "pattern:" + p.ID }

// IsGeneric reports a generic pattern.
func (p *Pattern) IsGeneric() bool { return strings.HasPrefix(p.ID, "g:") }

// Share is the pattern's share of the problem clusters, in percent.
func (d *Data) Share(p *Pattern) float64 {
	if d.Clusters == 0 {
		return 0
	}
	return 100 * float64(p.NClusters) / float64(d.Clusters)
}

// Seen says how often the pattern occurs: "2,248 of 6,734 public 1.6 logs".
func (d *Data) Seen(p *Pattern) string {
	return fmt.Sprintf("%s of %s public %s logs", Thousands(p.Logs), Thousands(d.Logs), d.Game)
}

// minPairs: fewer consecutive-upload pairs than this say nothing.
const minPairs = 5

// Line summarises the session evidence, or "" when there are too few pairs: "after 1-3 list
// changes it went away in 19% of 143 repeat uploads (without any change: 6% of 269)". What
// changed in those uploads is FixList(r.Fixes).
func (r *Resolution) Line() string {
	if r == nil || r.Pairs["small"] < minPairs {
		return ""
	}
	s := fmt.Sprintf("after 1-3 list changes it went away in %s of %d repeat uploads", pct(r.Gone["small"], r.Pairs["small"]), r.Pairs["small"])
	if r.Pairs["none"] >= minPairs {
		s += fmt.Sprintf(" (without any change: %s of %d)", pct(r.Gone["none"], r.Pairs["none"]), r.Pairs["none"])
	}
	return s
}

// FixList formats fix counts, most frequent first: "disable-other 15, enable 14".
func FixList(m map[string]int) string {
	type kv struct {
		k string
		v int
	}
	var s []kv
	for k, v := range m {
		s = append(s, kv{k, v})
	}
	sort.Slice(s, func(a, b int) bool { return s[a].v > s[b].v || s[a].v == s[b].v && s[a].k < s[b].k })
	var out []string
	for _, x := range s {
		out = append(out, fmt.Sprintf("%s %d", x.k, x.v))
	}
	return strings.Join(out, ", ")
}

func pct(a, b int) string {
	if b == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(a)/float64(b))
}

// Thousands writes n with comma separators.
func Thousands(n int) string {
	s := fmt.Sprint(n)
	if n < 0 {
		return "-" + Thousands(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
