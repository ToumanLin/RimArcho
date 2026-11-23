package app

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Between checks (05 §4.4): the patch simulation is the analysis's heaviest step, so
// after a list edit the stored run is stale for tens of seconds. The one patch finding a
// player can act on, a patch whose target Mod is not enabled (target-inactive), follows
// the list at once from the last run's operation facts and the Def index:
//
//   - a stored target-inactive no-match whose target Mods are all enabled now is dropped;
//   - an operation that succeeded in the last run, unguarded (analyze.ReasonGuarded), whose
//     every touched def is now defined only by installed Mods that are not enabled, gets a
//     target-inactive no-match like the analysis would give it.
//
// Operations of Mods the last run did not have, defs that only a patch creates, and the
// other reasons wait for the check. The findings carry the key prefix quickPrefix and are
// replaced by the analysis's own when it is fresh.

const quickPrefix = "quick|"

type quickFacts struct {
	ops      []*analyze.PatchFact // succeeded, unguarded, with touched defs
	definers map[string][]string  // "ThingDef/Gun_A" → instances that define it
	byInst   map[string][]*analyze.PatchFact
}

var quickCache struct {
	sync.Mutex
	keys []string
	m    map[string]*quickFacts
}

// patchFactsForQuick reads (once per run, index and game version) what the quick check
// needs.
func (a *App) patchFactsForQuick(runID int64) (*quickFacts, error) {
	key := fmt.Sprintf("%s|%d|%s|%s", a.Home, runID, a.Store.IndexKey(), a.Store.MM)
	quickCache.Lock()
	if q := quickCache.m[key]; q != nil {
		quickCache.Unlock()
		return q, nil
	}
	quickCache.Unlock()
	facts, err := a.Store.PatchFacts(runID, store.PatchFilter{})
	if err != nil {
		return nil, err
	}
	q := &quickFacts{definers: map[string][]string{}, byInst: map[string][]*analyze.PatchFact{}}
	for _, f := range facts {
		// at most 50 touched defs are recorded: a full list may hide one still defined
		if f.Result != analyze.ResultOK || len(f.Targets) == 0 || len(f.Targets) >= 50 || slices.Contains(f.Reasons, analyze.ReasonGuarded) {
			continue
		}
		q.ops = append(q.ops, f)
		q.byInst[f.Instance] = append(q.byInst[f.Instance], f)
		for _, t := range f.Targets {
			if _, ok := q.definers[t]; ok {
				continue
			}
			q.definers[t], err = a.defDefiners(t)
			if err != nil {
				return nil, err
			}
		}
	}
	quickCache.Lock()
	if quickCache.m == nil {
		quickCache.m = map[string]*quickFacts{}
	}
	quickCache.m[key] = q
	quickCache.keys = append(quickCache.keys, key)
	if len(quickCache.keys) > 4 {
		delete(quickCache.m, quickCache.keys[0])
		quickCache.keys = quickCache.keys[1:]
	}
	quickCache.Unlock()
	return q, nil
}

// defDefiners are the installed instances defining a touched def ("Type/defName" or
// "Type@Name").
func (a *App) defDefiners(target string) ([]string, error) {
	typ, name, byName := target, "", false
	if i := strings.IndexByte(target, '/'); i > 0 {
		typ, name = target[:i], target[i+1:]
	} else if i := strings.IndexByte(target, '@'); i > 0 {
		typ, name, byName = target[:i], target[i+1:], true
	} else {
		return nil, nil
	}
	defs, err := a.Store.DefsNamed(name, byName)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, d := range defs {
		if d.Type == typ && !slices.Contains(out, d.Instance) {
			out = append(out, d.Instance)
		}
	}
	return out, nil
}

// quickPatchTargets adjusts the stored findings all for the list st when the run they
// come from is stale (see above). It returns the new list.
func (a *App) quickPatchTargets(st *LoadoutState, an *AnalysisState, all []*finding.Finding) []*finding.Finding {
	if an == nil || an.Run == nil || an.Fresh || !an.Run.Stats.Guards {
		return all // fresh, or a run from before guarded operations were marked
	}
	enabled := func(id string) bool { return id != "" && st.Position(id) > 0 }
	out := all[:0:0]
	for _, f := range all {
		if f.Code == "patch.no-match" && (strings.HasPrefix(f.Key, quickPrefix) || targetsNowEnabled(f, enabled)) {
			continue
		}
		out = append(out, f)
	}
	q, err := a.patchFactsForQuick(an.Run.ID)
	if err != nil {
		return out // the check will tell
	}
	for _, f := range q.ops {
		if !enabled(f.Instance) || st.Ix.ByID[f.Instance] == nil {
			continue
		}
		var missing []string
		gone := true
		for _, t := range f.Targets {
			ds := q.definers[t]
			if len(ds) == 0 || slices.ContainsFunc(ds, enabled) {
				gone = false // a patch made it, or an enabled Mod still defines it
				break
			}
			for _, d := range ds {
				if st.Ix.ByID[d] != nil && !slices.Contains(missing, d) {
					missing = append(missing, d)
				}
			}
		}
		if !gone || len(missing) == 0 {
			continue
		}
		g := &finding.Finding{Code: "patch.no-match", Severity: finding.Warning, Layer: "patch", Confidence: finding.Likely,
			Key:      quickPrefix + f.File + "|" + f.Desc,
			Subjects: []string{finding.Inst(f.Instance)},
			Claim:    fmt.Sprintf("%s:%d %s matched nothing (%s)", f.File, f.Line, f.Desc, analyze.ReasonTargetInactive)}
		g.Evidence = append(g.Evidence, finding.Evidence{Source: finding.Inferred, Text: "reason " + analyze.ReasonTargetInactive},
			finding.Evidence{Source: finding.Inferred, Text: "from the last check and the list as it is now; the next check confirms it"})
		for _, d := range missing {
			g.Subjects = append(g.Subjects, finding.Inst(d))
			g.Suggestions = append(g.Suggestions, "archo enable "+finding.Inst(d))
		}
		out = append(out, g)
	}
	return out
}

// targetsNowEnabled: a stored target-inactive no-match whose inactive targets are all
// enabled now.
func targetsNowEnabled(f *finding.Finding, enabled func(string) bool) bool {
	if len(f.Subjects) < 2 {
		return false
	}
	inactive := false
	for _, e := range f.Evidence {
		if e.Text == "reason "+analyze.ReasonTargetInactive {
			inactive = true
		}
	}
	if !inactive {
		return false
	}
	for _, s := range f.Subjects[1:] {
		if !enabled(finding.InstID(s)) {
			return false
		}
	}
	return true
}

// quickPatchIncremental is quickPatchTargets for IncrementalProblems, whose content
// problems are copies of the previous set's: the patch ones are worked out again for the
// list as it is now, through the same ignores.
func (a *App) quickPatchIncremental(ps *ProblemSet) {
	if ps.Analysis == nil || ps.Analysis.Run == nil || ps.Analysis.Fresh || !ps.Analysis.Run.Stats.Guards {
		return
	}
	var stored []*finding.Finding
	keep := func(list []*Problem) []*Problem {
		out := list[:0:0]
		for _, p := range list {
			if p.Code == "patch.no-match" && p.Log == nil {
				if !strings.HasPrefix(p.Key, quickPrefix) {
					stored = append(stored, p.Finding)
				}
				continue
			}
			out = append(out, p)
		}
		return out
	}
	ps.List, ps.Ignored = keep(ps.List), keep(ps.Ignored)
	adjusted := a.quickPatchTargets(ps.State, ps.Analysis, stored)
	tmp := &ProblemSet{State: ps.State}
	if err := a.finishProblems(tmp, adjusted, nil, nil); err != nil {
		return
	}
	ps.List = append(ps.List, tmp.List...)
	ps.Ignored = append(ps.Ignored, tmp.Ignored...)
}
