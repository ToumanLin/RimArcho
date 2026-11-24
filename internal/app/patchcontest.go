package app

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/order"
)

// Same-target patches (08 §1.6): two enabled Mods whose XML patches replace or remove the
// same node. The game merges every Mod's Defs before it applies any patch, so a Mod that
// patches another's Defs works in either order and sorting adds no constraint for it; only
// here does the order decide something, the later Mod's change wins. They are shown, never
// sorted for: the stored analysis's patch.contested findings, read again for the list as it
// is now. Sorting never starts an analysis.

// Why there are no same-target notes (SortResult.ContestNote).
const ContestNoAnalysis = "no stored analysis of this list's Mods: same-target patches not checked (archo analyze)"

// PatchContest is one node that two or more enabled Mods replace or remove.
type PatchContest struct {
	Node string   // "ThingDef/Gun_A/statBases/MarketValue" (05 §4.3 patch.contested key)
	Mods []string // the enabled instances, in no particular order
}

// Winner is the Mod loaded last in l, whose change the game keeps ("" when fewer than two
// are in l).
func (c PatchContest) Winner(l loadout.List) string {
	best, at := "", -1
	for _, m := range c.Mods {
		if i := l.Index(m); i > at {
			best, at = m, i
		}
	}
	return best
}

// ContestedRef is the ignore reference that hides one pair's same-target notes: both
// lineages, sorted, so either order of the pair names the same rule.
func ContestedRef(linA, linB string) string {
	if linB < linA {
		linA, linB = linB, linA
	}
	return "contested:" + linA + ":" + linB
}

var contestCache struct {
	sync.Mutex
	key string
	fs  []*finding.Finding
}

// storedContests are a run's patch.contested findings, read once per run.
func (a *App) storedContests(runID int64) ([]*finding.Finding, error) {
	key := fmt.Sprintf("%s|%d", a.Home, runID)
	contestCache.Lock()
	if contestCache.key == key {
		fs := contestCache.fs
		contestCache.Unlock()
		return fs, nil
	}
	contestCache.Unlock()
	all, err := a.Store.Findings(runID)
	if err != nil {
		return nil, err
	}
	var fs []*finding.Finding
	for _, f := range all {
		if f.Code == codeContested {
			fs = append(fs, f)
		}
	}
	contestCache.Lock()
	contestCache.key, contestCache.fs = key, fs
	contestCache.Unlock()
	return fs, nil
}

const codeContested = "patch.contested"

// contestIgnores are the lineage pairs hidden by `rule add ignore contested:…`.
func (a *App) contestIgnores(st *LoadoutState) (map[string]bool, error) {
	rules, err := a.Store.Rules(st.Ix.Env.ID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, r := range rules {
		if r.Kind == order.RuleIgnore && strings.HasPrefix(r.Target, "contested:") {
			out[r.Target] = true
		}
	}
	return out, nil
}

// contestOf reads one stored finding for the list l: its enabled Mods, unless fewer than
// two are enabled or every pair among them is ignored.
func contestOf(st *LoadoutState, f *finding.Finding, l loadout.List, ignored map[string]bool) (PatchContest, bool) {
	c := PatchContest{Node: f.Key}
	for _, s := range f.Subjects {
		if id := finding.InstID(s); id != "" && l.Index(id) >= 0 && st.Ix.ByID[id] != nil && !slices.Contains(c.Mods, id) {
			c.Mods = append(c.Mods, id)
		}
	}
	if len(c.Mods) < 2 {
		return c, false
	}
	for i, x := range c.Mods {
		for _, y := range c.Mods[i+1:] {
			if !ignored[ContestedRef(st.Ix.ByID[x].LineageID, st.Ix.ByID[y].LineageID)] {
				return c, true
			}
		}
	}
	return c, false
}

// PatchContests are the same-target patches of the list l from the stored analysis of st's
// slot (or the environment's newest run); note says why there are none when no run is stored.
func (a *App) PatchContests(st *LoadoutState, l loadout.List) ([]PatchContest, string, error) {
	an, err := a.Analysis(st)
	if err != nil {
		return nil, "", err
	}
	if an.Run == nil {
		return nil, ContestNoAnalysis, nil
	}
	fs, err := a.storedContests(an.Run.ID)
	if err != nil {
		return nil, "", err
	}
	ignored, err := a.contestIgnores(st)
	if err != nil {
		return nil, "", err
	}
	var out []PatchContest
	for _, f := range fs {
		if c, ok := contestOf(st, f, l, ignored); ok {
			out = append(out, c)
		}
	}
	return out, "", nil
}

// ContestWhat is a node key in words: "ThingDef Gun_A's statBases/MarketValue", or
// "ThingDef Gun_A" for the whole Def.
func ContestWhat(node string) string {
	typ, rest := node, ""
	if i := strings.IndexAny(node, "/@"); i > 0 {
		typ, rest = node[:i], node[i+1:]
	}
	name, field, _ := strings.Cut(rest, "/")
	if field == "" {
		return typ + " " + name
	}
	return typ + " " + name + "'s " + field
}

// ContestClaim says who changes the node and who wins in l (instance tokens).
func ContestClaim(c PatchContest, l loadout.List) string {
	who := make([]string, 0, len(c.Mods))
	ordered := slices.Clone(c.Mods)
	slices.SortFunc(ordered, func(x, y string) int { return l.Index(x) - l.Index(y) })
	for _, m := range ordered {
		who = append(who, finding.Inst(m))
	}
	both := "both"
	if len(who) > 2 {
		both = "all"
	}
	names := strings.Join(who[:len(who)-1], ", ") + " and " + who[len(who)-1]
	return fmt.Sprintf("%s %s change %s; the one loaded later wins: currently %s", names, both, ContestWhat(c.Node), finding.Inst(c.Winner(l)))
}

// ContestActions pin the current winner with a user rule, or hide the pair: commands with
// instance tokens.
func ContestActions(st *LoadoutState, c PatchContest, l loadout.List) []string {
	w := c.Winner(l)
	prev, at := "", -1
	for _, m := range c.Mods {
		if i := l.Index(m); m != w && i > at {
			prev, at = m, i
		}
	}
	if prev == "" {
		return nil
	}
	return []string{
		fmt.Sprintf("archo rule add after %s %s", finding.Inst(w), finding.Inst(prev)),
		"archo rule add ignore contested:" + finding.Inst(w) + ":" + finding.Inst(prev),
	}
}

// contestFindings rewrites the stored patch.contested findings for the list as it is now
// (who wins depends on the current order, 08 §1.6) and drops those with fewer than two
// enabled Mods or only ignored pairs. The stored subjects stay, so the problem's reference
// does not change.
func (a *App) contestFindings(st *LoadoutState, all []*finding.Finding) []*finding.Finding {
	ignored, err := a.contestIgnores(st)
	if err != nil {
		return all
	}
	out := all[:0:0]
	for _, f := range all {
		if f.Code != codeContested {
			out = append(out, f)
			continue
		}
		c, ok := contestOf(st, f, st.List, ignored)
		if !ok {
			continue
		}
		g := *f
		g.Claim = ContestClaim(c, st.List)
		g.Suggestions = ContestActions(st, c, st.List)
		out = append(out, &g)
	}
	return out
}

// contestIncremental is contestFindings for IncrementalProblems' copies of the previous
// set's content problems.
func (a *App) contestIncremental(ps *ProblemSet) {
	redo := func(list []*Problem) []*Problem {
		out := list[:0:0]
		for _, p := range list {
			if p.Code == codeContested && p.Log == nil {
				fs := a.contestFindings(ps.State, []*finding.Finding{p.Finding})
				if len(fs) == 0 {
					continue
				}
				q := *p
				q.Finding = fs[0]
				p = &q
			}
			out = append(out, p)
		}
		return out
	}
	ps.List, ps.Ignored = redo(ps.List), redo(ps.Ignored)
}
