// Package order sorts a Loadout under hard constraints (04 §4): official order, dependencies,
// About.xml load rules, community rules and user rules, keeping the current relative order
// wherever the constraints allow. Constraint cycles are reported, never broken silently.
package order

import (
	"sort"
	"strings"

	"github.com/rimarcho/rimarcho/internal/scan"
)

// Official is the built-in order of Core and the DLCs (by release).
var Official = []string{"ludeon.rimworld", "ludeon.rimworld.royalty", "ludeon.rimworld.ideology",
	"ludeon.rimworld.biotech", "ludeon.rimworld.anomaly", "ludeon.rimworld.odyssey"}

func officialRank(pkg string) int {
	for i, p := range Official {
		if p == pkg {
			return i
		}
	}
	if strings.HasPrefix(pkg, "ludeon.rimworld.") {
		return len(Official) // unknown future DLC: after the known ones
	}
	return -1
}

// Node is one enabled entry, in current order.
type Node struct {
	Instance  string // "" for an entry that matched no instance
	PackageID string
	Lineage   string
	Decl      *scan.Decl
}

// Rule kinds (04 §5.1).
const (
	RuleAfter        = "after"
	RuleBefore       = "before"
	RuleIncompatible = "incompatible"
	RuleAttach       = "attach"
	RuleIgnore       = "ignore"
)

// Rule is a user rule; A and B are lineage IDs, Target an ignore reference.
type Rule struct {
	ID     int64
	Kind   string
	A, B   string
	Target string
}

// Community supplies community ordering rules by packageId (04 §5.2).
type Community interface {
	Rules(pkg string) (loadAfter, loadBefore []string)
}

// Input to Sort.
type Input struct {
	Nodes     []Node
	GameMM    string // "1.6": selects *ByVersion entries
	Rules     []Rule
	Community Community
}

// Constraint kinds.
const (
	KindOfficial   = "official"
	KindDependency = "dependency"
	KindLoadBefore = "loadBefore"
	KindLoadAfter  = "loadAfter"
	KindForceBef   = "forceLoadBefore"
	KindForceAft   = "forceLoadAfter"
	KindCommunity  = "community"
	KindUser       = "user"
)

// Constraint records where an edge came from, for cycle reports and `ignore` rules.
type Constraint struct {
	Kind   string
	Owner  int    // node whose declaration or package produced the edge (-1: none)
	Field  string // About.xml field, community field, or user rule kind
	Target string // the packageId named by the declaration
	Ref    string // ignore reference (stored form)
	RuleID int64
}

// Edge says node From must load before node To.
type Edge struct {
	From, To int
	C        Constraint
}

// Cycle is a strongly connected set of nodes with the edges that bind it.
type Cycle struct {
	Members []int
	Edges   []Edge
}

// Result of Sort.
type Result struct {
	Order   []int // node indices in the new order
	Cycles  []Cycle
	Edges   []Edge
	Ignored int // edges removed by ignore rules
}

// DeclRef is the ignore reference of an About.xml constraint, keyed by lineage so it
// survives package renames and copies.
func DeclRef(lineage, field, target string) string {
	return "decl:" + lineage + ":" + strings.ToLower(field) + ":" + strings.ToLower(target)
}

// CommunityRef is the ignore reference of a community rule.
func CommunityRef(pkg, field, target string) string {
	return "community:" + pkg + ":" + strings.ToLower(field) + ":" + strings.ToLower(target)
}

// OfficialRef is the ignore reference of the built-in official order for one mod.
func OfficialRef(lineage string) string { return "official:" + lineage }

func withVersion(base []string, byV map[string][]string, mm string) []string {
	out := append([]string(nil), base...)
	if mm != "" {
		out = append(out, byV["v"+mm]...)
	}
	return out
}

// Sort computes the new order (04 §4.2).
func Sort(in Input) Result {
	n := len(in.Nodes)
	byPkg := map[string][]int{}
	byLin := map[string][]int{}
	for i, nd := range in.Nodes {
		if nd.PackageID != "" {
			byPkg[nd.PackageID] = append(byPkg[nd.PackageID], i)
		}
		if nd.Lineage != "" {
			byLin[nd.Lineage] = append(byLin[nd.Lineage], i)
		}
	}
	ignore := map[string]bool{}
	for _, r := range in.Rules {
		if r.Kind == RuleIgnore {
			ignore[r.Target] = true
		}
	}
	var res Result
	seen := map[[2]int]bool{}
	add := func(from, to int, c Constraint) {
		if from == to {
			return
		}
		if c.Ref != "" && ignore[c.Ref] {
			res.Ignored++
			return
		}
		res.Edges = append(res.Edges, Edge{From: from, To: to, C: c})
		seen[[2]int{from, to}] = true
	}
	pkgsOf := func(l []string) []string {
		out := make([]string, 0, len(l))
		for _, p := range l {
			out = append(out, strings.ToLower(strings.TrimSpace(p)))
		}
		return out
	}

	// Mods that explicitly load before an official package (Harmony) are exempt from the
	// built-in "official first" order; otherwise every such list would be a cycle.
	exempt := make([]bool, n)
	for i, nd := range in.Nodes {
		if nd.Decl == nil {
			continue
		}
		for _, p := range pkgsOf(append(withVersion(nd.Decl.LoadBefore, nd.Decl.LoadBeforeByVersion, in.GameMM), nd.Decl.ForceLoadBefore...)) {
			if officialRank(p) >= 0 {
				exempt[i] = true
			}
		}
	}
	// Built-in: official content first, in official order.
	var officials []int
	for i, nd := range in.Nodes {
		if officialRank(nd.PackageID) >= 0 {
			officials = append(officials, i)
		}
	}
	sort.SliceStable(officials, func(a, b int) bool {
		return officialRank(in.Nodes[officials[a]].PackageID) < officialRank(in.Nodes[officials[b]].PackageID)
	})
	for k := 1; k < len(officials); k++ {
		o := in.Nodes[officials[k]]
		add(officials[k-1], officials[k], Constraint{Kind: KindOfficial, Owner: officials[k], Field: "official-order", Target: in.Nodes[officials[k-1]].PackageID, Ref: OfficialRef(o.Lineage)})
	}
	if len(officials) > 0 {
		last := officials[len(officials)-1]
		for i, nd := range in.Nodes {
			if officialRank(nd.PackageID) >= 0 || exempt[i] {
				continue
			}
			add(last, i, Constraint{Kind: KindOfficial, Owner: i, Field: "official-first", Target: in.Nodes[last].PackageID, Ref: OfficialRef(nd.Lineage)})
		}
	}
	// About.xml declarations.
	for i, nd := range in.Nodes {
		d := nd.Decl
		if d == nil {
			continue
		}
		deps := map[string]bool{}
		for _, x := range d.Dependencies {
			deps[strings.ToLower(x.PackageID)] = true
		}
		for _, x := range d.DependenciesByVersion["v"+in.GameMM] {
			deps[strings.ToLower(x.PackageID)] = true
		}
		depList := make([]string, 0, len(deps))
		for p := range deps {
			depList = append(depList, p)
		}
		sort.Strings(depList)
		for _, p := range depList {
			for _, j := range byPkg[p] {
				add(j, i, Constraint{Kind: KindDependency, Owner: i, Field: "modDependencies", Target: p, Ref: DeclRef(nd.Lineage, "dependency", p)})
			}
		}
		for _, f := range []struct {
			kind, field string
			list        []string
			before      bool
		}{
			{KindLoadBefore, "loadBefore", withVersion(d.LoadBefore, d.LoadBeforeByVersion, in.GameMM), true},
			{KindLoadAfter, "loadAfter", withVersion(d.LoadAfter, d.LoadAfterByVersion, in.GameMM), false},
			{KindForceBef, "forceLoadBefore", d.ForceLoadBefore, true},
			{KindForceAft, "forceLoadAfter", d.ForceLoadAfter, false},
		} {
			for _, p := range pkgsOf(f.list) {
				for _, j := range byPkg[p] {
					c := Constraint{Kind: f.kind, Owner: i, Field: f.field, Target: p, Ref: DeclRef(nd.Lineage, f.field, p)}
					if f.before {
						add(i, j, c)
					} else {
						add(j, i, c)
					}
				}
			}
		}
	}
	// Community rules.
	if in.Community != nil {
		for i, nd := range in.Nodes {
			if nd.PackageID == "" {
				continue
			}
			after, before := in.Community.Rules(nd.PackageID)
			for _, p := range pkgsOf(after) {
				for _, j := range byPkg[p] {
					add(j, i, Constraint{Kind: KindCommunity, Owner: i, Field: "loadAfter", Target: p, Ref: CommunityRef(nd.PackageID, "loadAfter", p)})
				}
			}
			for _, p := range pkgsOf(before) {
				for _, j := range byPkg[p] {
					add(i, j, Constraint{Kind: KindCommunity, Owner: i, Field: "loadBefore", Target: p, Ref: CommunityRef(nd.PackageID, "loadBefore", p)})
				}
			}
		}
	}
	// User rules: hard, between lineages.
	attach := map[int][]int{} // target node → nodes that want to follow it immediately
	for _, r := range in.Rules {
		for _, a := range byLin[r.A] {
			for _, b := range byLin[r.B] {
				c := Constraint{Kind: KindUser, Owner: a, Field: r.Kind, Target: in.Nodes[b].PackageID, RuleID: r.ID}
				switch r.Kind {
				case RuleAfter:
					add(b, a, c)
				case RuleBefore:
					add(a, b, c)
				case RuleAttach:
					add(b, a, c)
					attach[b] = append(attach[b], a)
				}
			}
		}
	}

	adj := make([][]int, n)
	for k := range seen {
		adj[k[0]] = append(adj[k[0]], k[1])
	}
	for i := range adj {
		sort.Ints(adj[i])
	}
	comp := tarjan(n, adj)
	nComp := 0
	for _, c := range comp {
		nComp = max(nComp, c+1)
	}
	members := make([][]int, nComp)
	for i, c := range comp {
		members[c] = append(members[c], i) // ascending node order
	}
	for c, ms := range members {
		if len(ms) < 2 {
			continue
		}
		cy := Cycle{Members: ms}
		for _, e := range res.Edges {
			if comp[e.From] == c && comp[e.To] == c {
				cy.Edges = append(cy.Edges, e)
			}
		}
		res.Cycles = append(res.Cycles, cy)
	}
	sort.Slice(res.Cycles, func(a, b int) bool { return res.Cycles[a].Members[0] < res.Cycles[b].Members[0] })

	// Kahn over components: pick the ready component whose first member is earliest in the
	// current list, preferring one attached to what was just emitted.
	indeg := make([]int, nComp)
	cadj := make([]map[int]bool, nComp)
	for i := range cadj {
		cadj[i] = map[int]bool{}
	}
	for k := range seen {
		a, b := comp[k[0]], comp[k[1]]
		if a != b && !cadj[a][b] {
			cadj[a][b] = true
			indeg[b]++
		}
	}
	key := func(c int) int { return members[c][0] }
	ready := map[int]bool{}
	for c := 0; c < nComp; c++ {
		if indeg[c] == 0 {
			ready[c] = true
		}
	}
	boost := map[int]bool{}
	for len(ready) > 0 {
		pick := -1
		for c := range ready {
			switch {
			case pick < 0:
				pick = c
			case boost[c] != boost[pick]:
				if boost[c] {
					pick = c
				}
			case key(c) < key(pick):
				pick = c
			}
		}
		delete(ready, pick)
		boost = map[int]bool{}
		res.Order = append(res.Order, members[pick]...)
		for _, m := range members[pick] {
			for _, a := range attach[m] {
				boost[comp[a]] = true
			}
		}
		for d := range cadj[pick] {
			indeg[d]--
			if indeg[d] == 0 {
				ready[d] = true
			}
		}
	}
	return res
}

// tarjan returns the strongly connected component of every node.
func tarjan(n int, adj [][]int) []int {
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	comp := make([]int, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	next, nComp := 0, 0
	var visit func(v int)
	visit = func(v int) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range adj[v] {
			if index[w] < 0 {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp[w] = nComp
				if w == v {
					break
				}
			}
			nComp++
		}
	}
	for v := 0; v < n; v++ {
		if index[v] < 0 {
			visit(v)
		}
	}
	return comp
}
