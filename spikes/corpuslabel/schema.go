package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Label is the fixed output of a labeller (design R2: structured, with model and prompt).
type Label struct {
	Schema string `json:"schema"` // "corpuslabel/1"
	Item   string `json:"item"`
	Gist   string `json:"gist"`
	Key    string `json:"key"`
	// Verdict: "cause" (a cause is named), "unknown" (the evidence does not decide),
	// "benign" (not a problem the player needs to act on).
	Verdict string  `json:"verdict"`
	Causes  []Cause `json:"causes"`
	// Stage: when it fails.
	Stage string `json:"stage"`
	// CauseType: why it fails.
	CauseType  string   `json:"cause_type"`
	Fix        Fix      `json:"fix"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"` // verbatim quotes from the packet
	Model      string   `json:"model"`
	Prompt     string   `json:"prompt"`
	Note       string   `json:"note,omitempty"`
}

// Cause is one mod of the cause. Role:
//
//	source   its own content or code is wrong or out of date (the mod to fix, update or drop)
//	partner  the other side of an incompatibility or ordering problem with a source
//	missing  a mod that should be active and is not (a dependency); pkg need not be in the list
type Cause struct {
	Pkg  string `json:"pkg"`
	Role string `json:"role"`
}

type Fix struct {
	Kind string   `json:"kind"`
	Mods []string `json:"mods,omitempty"`
	Note string   `json:"note,omitempty"`
}

var (
	verdicts   = set("cause", "unknown", "benign")
	roles      = set("source", "partner", "missing")
	stages     = set("assembly-load", "xml-load", "patch-apply", "def-resolve", "static-init", "game-start", "save-load", "map-gen", "tick", "ui", "other")
	causeTypes = set("missing-dependency", "load-order", "incompatible-mods", "outdated-mod", "game-version", "mod-bug", "missing-content", "other")
	fixKinds   = set("disable", "enable-dependency", "reorder", "update", "other", "none")
	confs      = set("high", "medium", "low")
	pkgRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9_.\-]*$`)
)

func set(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func keys(m map[string]bool) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, "|")
}

// validate checks a label against its sample item and packet. Referenced gist ids and
// cluster keys must exist; packageIds must be in the log's list (or, for role missing, at
// least appear in the packet text); quotes must be verbatim.
func validate(l *Label, items map[string]Item, packets map[string]*Packet) []string {
	var errs []string
	bad := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }
	if l.Schema != "corpuslabel/1" {
		bad("schema %q", l.Schema)
	}
	it, ok := items[l.Item]
	if !ok {
		bad("item %q not in the sample", l.Item)
		return errs
	}
	p := packets[l.Item]
	if l.Gist != it.Gist {
		bad("gist %q is not the item's", l.Gist)
	}
	if p == nil {
		bad("no packet")
		return errs
	}
	if l.Key != p.Key && l.Key != it.Key {
		bad("cluster key does not match the item")
	}
	if !verdicts[l.Verdict] {
		bad("verdict %q not in %s", l.Verdict, keys(verdicts))
	}
	inList := map[string]bool{}
	for _, pk := range p.LogMods {
		inList[pk] = true
	}
	lowText := strings.ToLower(p.Text)
	for _, c := range l.Causes {
		if !roles[c.Role] {
			bad("role %q not in %s", c.Role, keys(roles))
		}
		pk := strings.ToLower(c.Pkg)
		switch {
		case !pkgRe.MatchString(pk):
			bad("packageId %q malformed", c.Pkg)
		case c.Role == "missing" && !strings.Contains(lowText, pk):
			bad("missing mod %q appears nowhere in the packet", c.Pkg)
		case c.Role != "missing" && !inList[pk]:
			bad("packageId %q not in the log's mod list", c.Pkg)
		}
	}
	if l.Verdict == "cause" && len(l.Causes) == 0 {
		bad("verdict cause without causes")
	}
	if !stages[l.Stage] {
		bad("stage %q not in %s", l.Stage, keys(stages))
	}
	if !causeTypes[l.CauseType] {
		bad("cause_type %q not in %s", l.CauseType, keys(causeTypes))
	}
	if !fixKinds[l.Fix.Kind] {
		bad("fix.kind %q not in %s", l.Fix.Kind, keys(fixKinds))
	}
	for _, m := range l.Fix.Mods {
		if pk := strings.ToLower(m); !inList[pk] && !strings.Contains(lowText, pk) {
			bad("fix mod %q not in the list or packet", m)
		}
	}
	if !confs[l.Confidence] {
		bad("confidence %q not in %s", l.Confidence, keys(confs))
	}
	if len(l.Evidence) == 0 {
		bad("no evidence quotes")
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	nt := norm(p.Text)
	for _, q := range l.Evidence {
		// A reviewer may cite what the packet lacks: "log: …" (raw log), "env: …" (research
		// environment lookup), "corpus: …" (co-occurrence). Those are not checked here.
		if strings.HasPrefix(q, "log: ") || strings.HasPrefix(q, "env: ") || strings.HasPrefix(q, "corpus: ") {
			continue
		}
		if !strings.Contains(nt, norm(strings.TrimSuffix(q, "…"))) {
			bad("quote not verbatim: %.60q", q)
		}
	}
	if l.Model == "" || l.Prompt == "" {
		bad("model and prompt are required")
	}
	return errs
}

func readLabels(dir string) (map[string]*Label, map[string][]string, error) {
	out := map[string]*Label{}
	parseErr := map[string][]string{}
	fs, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, nil, err
	}
	for _, f := range fs {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		var l Label
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		if err := dec.Decode(&l); err != nil {
			parseErr[name] = append(parseErr[name], "json: "+err.Error())
			continue
		}
		if l.Item == "" {
			l.Item = name
		}
		out[l.Item] = &l
	}
	return out, parseErr, nil
}

func readPackets(dir string) (map[string]*Packet, error) {
	out := map[string]*Packet{}
	fs, err := filepath.Glob(filepath.Join(dir, "P*.json"))
	if err != nil {
		return nil, err
	}
	for _, f := range fs {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var p Packet
		if err := json.Unmarshal(b, &p); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out[p.Item] = &p
	}
	return out, nil
}

func loadAll(samplePath, packetDir string) (map[string]Item, map[string]*Packet, error) {
	items, err := readSample(samplePath)
	if err != nil {
		return nil, nil, err
	}
	byItem := map[string]Item{}
	for _, it := range items {
		byItem[it.Item] = it
	}
	ps, err := readPackets(packetDir)
	return byItem, ps, err
}

func validateCmd(samplePath, packetDir, labelDir string) error {
	items, ps, err := loadAll(samplePath, packetDir)
	if err != nil {
		return err
	}
	ls, perr, err := readLabels(labelDir)
	if err != nil {
		return err
	}
	nBad := 0
	for name, es := range perr {
		nBad++
		fmt.Printf("%s: %s\n", name, strings.Join(es, "; "))
	}
	var names []string
	for n := range ls {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if es := validate(ls[n], items, ps); len(es) > 0 {
			nBad++
			fmt.Printf("%s: %s\n", n, strings.Join(es, "; "))
		}
	}
	fmt.Printf("%d labels, %d invalid, %d packets without a label\n", len(ls)+len(perr), nBad, len(ps)-len(ls))
	return nil
}
