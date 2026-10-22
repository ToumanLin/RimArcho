package main

// Export: the product's embedded pattern data (internal/patterns/patterns.json) from the
// research dataset, and a check that the product matcher classifies every corpus cluster as
// this spike does (01-范式库接入草案.md §5: research and product must agree).
//
//	go run ./spikes/patterns -export internal/patterns/patterns.json -dataset spikes/patterns/patterns-v0.json
//	go run ./spikes/patterns -verify <corpusrun results.jsonl>

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/patterns"
)

// export writes the product file: generic rules in match order (this spike's rules slice,
// first match wins) with their counts, then the mod-specific patterns (already filtered for
// privacy and frequency by run: >= minlogs logs from >= minsessions sessions, no paths).
func export(datasetPath, outPath string) error {
	b, err := os.ReadFile(datasetPath)
	if err != nil {
		return err
	}
	var ds struct {
		Version   string     `json:"version"`
		Game      string     `json:"game"`
		Logs      int        `json:"logs"`
		Generated string     `json:"generated"`
		Patterns  []*Pattern `json:"patterns"`
	}
	if err := json.Unmarshal(b, &ds); err != nil {
		return err
	}
	byID := map[string]*Pattern{}
	for _, p := range ds.Patterns {
		byID[p.ID] = p
	}
	res := func(r *Res) *patterns.Resolution {
		if r == nil || len(r.Pairs) == 0 {
			return nil
		}
		return &patterns.Resolution{Pairs: r.Pairs, Gone: r.Gone, Fixes: r.Fixes}
	}
	out := &patterns.Data{Version: ds.Version, Game: ds.Game, Logs: ds.Logs, Generated: ds.Generated}
	for _, r := range rules {
		p := byID["g:"+r.ID]
		if p == nil {
			return fmt.Errorf("dataset has no pattern g:%s", r.ID)
		}
		out.Clusters += p.Clusters
		out.Rules = append(out.Rules, &patterns.Pattern{ID: "g:" + r.ID, Class: r.Class, Match: r.Match, Fallback: r.Fallback,
			Stage: r.Stage, Summary: r.Summary, Causes: r.Causes, Route: r.Route, Fixes: r.Fixes, Workflow: r.Workflow,
			Logs: p.Logs, Sessions: p.Sessions, NClusters: p.Clusters, Res: res(p.Res)})
	}
	for _, p := range ds.Patterns {
		if p.Layer != "mod" {
			continue
		}
		if private(p.Template) {
			return fmt.Errorf("%s: template looks private", p.ID)
		}
		kind := ""
		if p.Rule != nil && p.Rule.ID != "" {
			kind = "g:" + p.Rule.ID
		}
		out.Mod = append(out.Mod, &patterns.Pattern{ID: p.ID, Class: p.Class, Template: p.Template, Mod: p.Mod, Kind: kind,
			Versions: p.Versions, First: p.First, Last: p.Last, Logs: p.Logs, Sessions: p.Sessions, NClusters: p.Clusters, Res: res(p.Res)})
	}
	// One pattern per line, as patterns-v0.json: readable diffs between dataset versions.
	var sb strings.Builder
	head := *out
	head.Rules, head.Mod = nil, nil
	hb, err := marshal(head)
	if err != nil {
		return err
	}
	sb.Write(hb[:len(hb)-1])
	list := func(name string, ps []*patterns.Pattern) error {
		sb.WriteString(",\n\"" + name + "\": [\n")
		for i, p := range ps {
			b, err := marshal(p)
			if err != nil {
				return err
			}
			sb.Write(b)
			if i < len(ps)-1 {
				sb.WriteString(",")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("]")
		return nil
	}
	if err := list("generic", out.Rules); err != nil {
		return err
	}
	if err := list("mod", out.Mod); err != nil {
		return err
	}
	sb.WriteString("}\n")
	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d generic, %d mod patterns, %d clusters, %d bytes\n", outPath, len(out.Rules), len(out.Mod), out.Clusters, sb.Len())
	return nil
}

// marshal is json.Marshal without HTML escaping (routes contain <n>) and without the newline.
func marshal(v any) ([]byte, error) {
	var b strings.Builder
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return []byte(strings.TrimSuffix(b.String(), "\n")), nil
}

// verify classifies every problem cluster of a corpusrun results file with this spike's rules
// and with the product matcher (embedded data) and counts disagreements.
func verify(resultsPath string) error {
	d := patterns.Load()
	n, diff := 0, 0
	shown := 0
	err := eachLine(resultsPath, func(b []byte) error {
		var r rresult
		if json.Unmarshal(b, &r) != nil {
			return nil
		}
		for _, c := range r.Clusters {
			if logs.Informational(c.Class) {
				continue
			}
			n++
			want := ""
			if g := classify(c.Class, c.Tpl); g != nil {
				want = "g:" + g.ID
			}
			got := ""
			if g := d.Generic(c.Class, c.Tpl); g != nil {
				got = g.ID
			}
			if want != got {
				diff++
				if shown < 10 {
					shown++
					fmt.Printf("  differ: spike %s, product %s: %s\n", want, got, trunc(c.Tpl, 120))
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("verify %s: %d problem clusters, %d classified differently\n", resultsPath, n, diff)
	if diff > 0 {
		return fmt.Errorf("%d clusters differ", diff)
	}
	return nil
}
