// Package rules holds community ordering data (04 §5.2). M2 reads a local RimSort-format
// communityRules.json if one is present; downloading (`archo data update`) comes later.
package rules

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// Community is a loaded community rule set, keyed by lower-case packageId.
type Community struct {
	Path      string
	Timestamp int64
	after     map[string][]string
	before    map[string][]string
}

// None is an empty rule set.
var None = &Community{}

// Rules implements order.Community.
func (c *Community) Rules(pkg string) (loadAfter, loadBefore []string) {
	if c == nil {
		return nil, nil
	}
	return c.after[pkg], c.before[pkg]
}

// Count returns the number of packages with rules.
func (c *Community) Count() int {
	if c == nil {
		return 0
	}
	n := map[string]bool{}
	for k := range c.after {
		n[k] = true
	}
	for k := range c.before {
		n[k] = true
	}
	return len(n)
}

// Load reads a RimSort communityRules.json:
//
//	{"timestamp": 1700000000, "rules": {"<pkg>": {"loadAfter": {"<pkg>": {...}}, "loadBefore": {...}}}}
//
// A missing file yields None and no error.
func Load(path string) (*Community, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return None, nil
	}
	if err != nil {
		return nil, err
	}
	var raw struct {
		Timestamp int64 `json:"timestamp"`
		Rules     map[string]struct {
			LoadAfter  map[string]json.RawMessage `json:"loadAfter"`
			LoadBefore map[string]json.RawMessage `json:"loadBefore"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	c := &Community{Path: path, Timestamp: raw.Timestamp, after: map[string][]string{}, before: map[string][]string{}}
	keys := func(m map[string]json.RawMessage) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, strings.ToLower(k))
		}
		sort.Strings(out)
		return out
	}
	for pkg, r := range raw.Rules {
		p := strings.ToLower(pkg)
		if l := keys(r.LoadAfter); len(l) > 0 {
			c.after[p] = l
		}
		if l := keys(r.LoadBefore); len(l) > 0 {
			c.before[p] = l
		}
	}
	return c, nil
}
