// Package rules holds community ordering data (08 §2.3). M2 reads a local RimSort-format
// communityRules.json if one is present; downloading (`archo data update`) comes later.
package rules

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/order"
)

// Community is a loaded community rule set, keyed by lower-case packageId.
type Community struct {
	Path      string
	Timestamp int64
	after     map[string][]string
	before    map[string][]string
	position  map[string]order.Tier // loadTop / loadBottom (08 §1.5)
}

// Position implements order.Positions.
func (c *Community) Position(pkg string) order.Tier {
	if c == nil {
		return order.TierNormal
	}
	return c.position[pkg]
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
	for k := range c.position {
		n[k] = true
	}
	return len(n)
}

// Load reads a RimSort communityRules.json:
//
//	{"timestamp": 1700000000, "rules": {"<pkg>": {"loadAfter": {"<pkg>": {...}}, "loadBefore": {...},
//	  "loadTop": {"value": true}, "loadBottom": {"value": true}}}}
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
	// Rules are decoded one package at a time: a malformed rule is skipped, never the
	// whole file; the timestamp may be a number or a quoted number.
	var raw struct {
		Timestamp json.RawMessage            `json:"timestamp"`
		Rules     map[string]json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	ts, _ := strconv.ParseFloat(strings.Trim(string(raw.Timestamp), `" `), 64)
	c := &Community{Path: path, Timestamp: int64(ts), after: map[string][]string{}, before: map[string][]string{}, position: map[string]order.Tier{}}
	keys := func(m map[string]json.RawMessage) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, strings.ToLower(k))
		}
		sort.Strings(out)
		return out
	}
	for pkg, body := range raw.Rules {
		var r struct {
			LoadAfter  map[string]json.RawMessage `json:"loadAfter"`
			LoadBefore map[string]json.RawMessage `json:"loadBefore"`
			LoadTop    *flag                      `json:"loadTop"`
			LoadBottom *flag                      `json:"loadBottom"`
		}
		if json.Unmarshal(body, &r) != nil {
			continue
		}
		p := strings.ToLower(pkg)
		if l := keys(r.LoadAfter); len(l) > 0 {
			c.after[p] = l
		}
		if l := keys(r.LoadBefore); len(l) > 0 {
			c.before[p] = l
		}
		switch {
		case r.LoadTop.on() && !r.LoadBottom.on():
			c.position[p] = order.TierTop
		case r.LoadBottom.on() && !r.LoadTop.on():
			c.position[p] = order.TierBottom
		}
	}
	return c, nil
}

// flag is RimSort's {"value": true, "comment": "..."}; the value may also be a string.
type flag struct {
	Value json.RawMessage `json:"value"`
}

func (f *flag) on() bool {
	if f == nil {
		return false
	}
	return strings.Trim(strings.ToLower(strings.TrimSpace(string(f.Value))), `"`) == "true"
}
