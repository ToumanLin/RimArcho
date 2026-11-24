package app

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Inferred rules (08 §1.6): a Mod whose XML patches change Defs that only one other enabled
// Mod defines prefers to load after that Mod. They come from the stored analysis only (the
// Patch facts' touched Defs and the Def facts of the enabled Mods); sorting never starts an
// analysis.

// Why a sort has no inferred rules (SortResult.InferredNote).
const (
	InferredNoAnalysis = "no stored analysis of this list's Mods: sorted without rules inferred from XML patches (archo analyze)"
	InferredOldRun     = "the stored analysis predates guarded patches: sorted without rules inferred from XML patches (archo analyze)"
)

// inferredOps are the touched Defs of a run's succeeded operations, by patching instance.
type inferredOps struct {
	byInst map[string][][]string // instance → each operation's touched defs
}

var inferredCache struct {
	sync.Mutex
	keys []string
	ops  map[string]*inferredOps
	defs map[string][]store.DefKey // "<home>|<index key>|<mm>|<instance>" → its Defs
}

// inferredOpsOf reads (once per run) the operations that touched Defs. An operation that
// touched 50 or more is a broad one (the analysis keeps 50 at most): it names no Mod.
func (a *App) inferredOpsOf(runID int64) (*inferredOps, error) {
	key := fmt.Sprintf("%s|%d|%s", a.Home, runID, a.Store.MM)
	inferredCache.Lock()
	if q := inferredCache.ops[key]; q != nil {
		inferredCache.Unlock()
		return q, nil
	}
	inferredCache.Unlock()
	facts, err := a.Store.PatchFacts(runID, store.PatchFilter{})
	if err != nil {
		return nil, err
	}
	q := &inferredOps{byInst: map[string][][]string{}}
	for _, f := range facts {
		if f.Result != analyze.ResultOK || len(f.Targets) == 0 || len(f.Targets) >= 50 {
			continue
		}
		q.byInst[f.Instance] = append(q.byInst[f.Instance], f.Targets)
	}
	inferredCache.Lock()
	if inferredCache.ops == nil {
		inferredCache.ops = map[string]*inferredOps{}
	}
	inferredCache.ops[key] = q
	inferredCache.keys = append(inferredCache.keys, key)
	if len(inferredCache.keys) > 4 {
		delete(inferredCache.ops, inferredCache.keys[0])
		inferredCache.keys = inferredCache.keys[1:]
	}
	inferredCache.Unlock()
	return q, nil
}

// defKeysOf is an instance's stored Def keys, cached per index state.
func (a *App) defKeysOf(inst string) ([]store.DefKey, error) {
	key := fmt.Sprintf("%s|%s|%s|%s", a.Home, a.Store.IndexKey(), a.Store.MM, inst)
	inferredCache.Lock()
	if d, ok := inferredCache.defs[key]; ok {
		inferredCache.Unlock()
		return d, nil
	}
	inferredCache.Unlock()
	d, err := a.Store.DefKeysOf(inst)
	if err != nil {
		return nil, err
	}
	inferredCache.Lock()
	if inferredCache.defs == nil || len(inferredCache.defs) > 4000 {
		inferredCache.defs = map[string][]store.DefKey{}
	}
	inferredCache.defs[key] = d
	inferredCache.Unlock()
	return d, nil
}

// InferredRules works out the inferred rules for the list l (08 §1.6) from the stored
// analysis of st's slot (or the environment's newest run). note says why there are none
// when no usable run is stored.
func (a *App) InferredRules(st *LoadoutState, l loadout.List) (rules []order.Inferred, note string, err error) {
	an, err := a.Analysis(st)
	if err != nil {
		return nil, "", err
	}
	if an.Run == nil {
		return nil, InferredNoAnalysis, nil
	}
	if !an.Run.Stats.Guards {
		return nil, InferredOldRun, nil
	}
	ops, err := a.inferredOpsOf(an.Run.ID)
	if err != nil {
		return nil, "", err
	}
	// The enabled Mods, their packages (MayRequire on a Def) and which are official.
	enabled := map[string]bool{}
	official := map[string]bool{}
	active := map[string]bool{}
	var insts []string
	for _, e := range l {
		m := st.Ix.ByID[e.Instance]
		if m == nil || enabled[m.ID] {
			continue
		}
		enabled[m.ID] = true
		insts = append(insts, m.ID)
		pkg := strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")
		active[pkg] = true
		official[m.ID] = strings.HasPrefix(pkg, "ludeon.rimworld")
	}
	// Who defines each Def among the enabled Mods; Defs the game skips (MayRequire of a
	// Mod not enabled) do not count.
	needed := map[string]bool{}
	for _, inst := range insts {
		for _, ts := range ops.byInst[inst] {
			for _, t := range ts {
				needed[t] = true
			}
		}
	}
	if len(needed) == 0 {
		return nil, "", nil
	}
	isActive := func(p string) bool { return active[p] }
	definers := map[string][]string{}
	for _, inst := range insts {
		keys, err := a.defKeysOf(inst)
		if err != nil {
			return nil, "", err
		}
		for _, k := range keys {
			for _, t := range []string{k.Type + "/" + k.DefName, k.Type + "@" + k.Name} {
				if t[len(k.Type)+1:] == "" || !needed[t] {
					continue
				}
				f := analyze.DefFact{MayRequire: k.MayRequire, MayRequireAny: k.MayRequireAny}
				if !f.Loaded(isActive) {
					continue
				}
				if ds := definers[t]; len(ds) == 0 || ds[len(ds)-1] != inst {
					definers[t] = append(definers[t], inst)
				}
			}
		}
	}
	type pair struct{ patcher, owner string }
	count := map[pair]int{}
	types := map[pair]map[string]int{}
	for _, inst := range insts {
		if official[inst] {
			continue
		}
		for _, ts := range ops.byInst[inst] {
			// The one Mod this operation changes: a Def defined by exactly one enabled Mod,
			// not official and not the patcher. Defs of official content, of several Mods
			// or created by patches name nobody; an operation naming two Mods is skipped.
			owner := ""
			byType := map[string]int{}
			for _, t := range ts {
				ds := definers[t]
				if len(ds) != 1 || ds[0] == inst || official[ds[0]] {
					continue
				}
				if owner != "" && owner != ds[0] {
					owner = "-"
					break
				}
				owner = ds[0]
				byType[defType(t)]++
			}
			if owner == "" || owner == "-" {
				continue
			}
			p := pair{inst, owner}
			count[p]++
			if types[p] == nil {
				types[p] = map[string]int{}
			}
			for t := range byType {
				types[p][t]++
			}
		}
	}
	for p, n := range count {
		inf := order.Inferred{Patcher: p.patcher, Owner: p.owner, Patches: n}
		for t := range types[p] {
			inf.DefTypes = append(inf.DefTypes, t)
		}
		ts := types[p]
		sort.Slice(inf.DefTypes, func(i, j int) bool {
			a, b := inf.DefTypes[i], inf.DefTypes[j]
			if ts[a] != ts[b] {
				return ts[a] > ts[b]
			}
			return a < b
		})
		rules = append(rules, inf)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Patcher != rules[j].Patcher {
			return rules[i].Patcher < rules[j].Patcher
		}
		return rules[i].Owner < rules[j].Owner
	})
	return rules, "", nil
}

// defType is the type of a touched-def key ("ThingDef/Gun_A", "ThingDef@BaseGun").
func defType(t string) string {
	if i := strings.IndexAny(t, "/@"); i > 0 {
		return t[:i]
	}
	return t
}
