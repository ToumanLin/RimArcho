package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// ImportRimSortRules reads userRules.json without modifying the source. Rules map
// only when each packageId resolves to exactly one live Lineage (07 §6.2).
func (a *App) ImportRimSortRules(path string, dryRun bool) (*RimCrowReport, error) {

	if st, err := os.Stat(path); err != nil {
		return nil, err
	} else if st.Size() > 4<<20 {
		return nil, errors.New("user rules exceed 4 MB")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) > 4<<20 {
		return nil, errors.New("user rules exceed 4 MB")
	}
	var raw struct {
		Rules map[string]map[string]json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	if raw.Rules == nil {
		return nil, errors.New("this is not a RimSort userRules.json file")
	}
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	existing, err := a.Store.Rules(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	rep := &RimCrowReport{Rules: path, DryRun: dryRun, NotMapped: map[string]int{}}
	resolve := func(pkg string) string {
		found := ""
		for _, m := range ix.Mods {
			if m.TombstoneAt.IsZero() && strings.EqualFold(strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam"), pkg) {
				if found != "" && found != m.LineageID {
					return ""
				}
				found = m.LineageID
			}
		}
		return found
	}
	packages := make([]string, 0, len(raw.Rules))
	for pkg := range raw.Rules {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)
	for _, pkg := range packages {
		kinds := make([]string, 0, len(raw.Rules[pkg]))
		for k := range raw.Rules[pkg] {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, field := range kinds {
			value := raw.Rules[pkg][field]
			kind := map[string]string{"loadAfter": order.RuleAfter, "loadBefore": order.RuleBefore, "incompatibleWith": order.RuleIncompatible, "loadTop": order.RuleTop, "loadBottom": order.RuleBottom}[field]
			if kind == "" {
				rep.NotMapped[field]++
				continue
			}
			targets := []string{}
			if field == "loadTop" || field == "loadBottom" {
				var flag struct {
					Value json.RawMessage `json:"value"`
				}
				if err := json.Unmarshal(value, &flag); err != nil {
					return nil, err
				}
				if strings.Trim(strings.ToLower(string(flag.Value)), `" `) != "true" {
					continue
				}
				targets = append(targets, "")
			} else {
				var refs map[string]json.RawMessage
				if err := json.Unmarshal(value, &refs); err != nil {
					return nil, err
				}
				for target := range refs {
					targets = append(targets, target)
				}
				sort.Strings(targets)
			}
			for _, target := range targets {
				rep.RulesTotal++
				la, lb := resolve(strings.ToLower(pkg)), resolve(strings.ToLower(target))
				label := strings.TrimSpace(pkg + " " + field + " " + target)
				if la == "" || target != "" && lb == "" {
					rep.RulesSkipped = append(rep.RulesSkipped, RCItem{label, "no single installed Mod matches this packageId"})
					continue
				}
				duplicate := false
				for _, r := range existing {
					if r.Kind == kind && r.A == la && r.B == lb {
						duplicate = true
						break
					}
				}
				if duplicate {
					rep.RuleSame++
					continue
				}
				r := &store.UserRule{Kind: kind, A: la, B: lb, Note: "imported from RimSort", Created: a.Now().UTC()}
				existing = append(existing, r)
				rep.RuleNew++
				if !dryRun {
					if _, err := a.Store.AddRule(ix.Env.ID, r); err != nil {
						return nil, fmt.Errorf("%s: %w", label, err)
					}
				}
			}
		}
	}
	return rep, nil
}
