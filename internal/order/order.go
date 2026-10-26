// Package order sorts a Loadout under hard constraints (04 §5): official order, dependencies,
// About.xml load rules, community rules and user rules, keeping the current relative order
// wherever the constraints allow. Constraint cycles are reported, never broken silently.
package order

import (
	"slices"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/scan"
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

// Rule kinds (04 §6.1).
const (
	RuleAfter        = "after"
	RuleBefore       = "before"
	RuleIncompatible = "incompatible"
	RuleAttach       = "attach"
	RuleIgnore       = "ignore"
	RuleTop          = "top"    // A loads as early as its constraints allow (04 §5.5)
	RuleBottom       = "bottom" // A loads as late as its constraints allow
)

// Rule is a user rule; A and B are lineage IDs, Target an ignore reference.
type Rule struct {
	ID     int64
	Kind   string
	A, B   string
	Target string
}

// Community supplies community ordering rules by packageId (04 §6.3).
type Community interface {
	Rules(pkg string) (loadAfter, loadBefore []string)
}

// Positions is implemented by a Community that also has loadTop / loadBottom (04 §5.5).
type Positions interface {
	Position(pkg string) Tier
}

// Tier is a preferred end of the list (04 §5.5). It only breaks ties between mods the
// constraints leave free: it never overrides a constraint.
type Tier int

const (
	TierTop    Tier = -1
	TierNormal Tier = 0
	TierBottom Tier = 1
)

func (t Tier) String() string {
	switch t {
	case TierTop:
		return "top"
	case TierBottom:
		return "bottom"
	}
	return "normal"
}

// Anchor is a mod asked to load at one end, with where the request came from.
type Anchor struct {
	Node   int
	Tier   Tier
	Kind   string // KindCommunity or KindUser
	Ref    string // community: the ignore reference
	RuleID int64  // user: the rule
	// Pulled: mods that go to that end with it (what it needs for top, what needs it for
	// bottom). Conflict: it is also pulled to the other end.
	Pulled   []int
	Conflict bool
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
	Members []int // in the sorted order
	Edges   []Edge
	Broken  []int // indices into Edges that the sorted order breaks (the weakest that had to give)
	Suggest int   // index into Edges of the edge suggested for an ignore rule; -1: none
}

// kindRank orders constraint kinds by how readily a cycle gives them up (04 §5.2): community
// rules, loadAfter, the other About.xml load rules, dependencies, the official order, user
// rules.
func kindRank(k string) int {
	switch k {
	case KindCommunity:
		return 0
	case KindLoadAfter:
		return 1
	case KindLoadBefore, KindForceAft, KindForceBef:
		return 2
	case KindDependency:
		return 3
	case KindOfficial:
		return 4
	}
	return 5 // user
}

// giveKey ranks an edge for giving up inside a cycle: lower gives first. Corpus strength
// (Assess) first, then the kind, then an edge the current order already breaks.
func giveKey(in Input, e Edge) int {
	k := int(Assess(in, e).Strength)*20 + kindRank(e.C.Kind)*2
	if e.From < e.To {
		k++ // the current order keeps it
	}
	return k
}

// suggestable: an ignore rule can drop it. User rules are never suggested — the user
// removes them instead.
func suggestable(e Edge) bool { return e.C.Kind != KindUser && e.C.Ref != "" }

// suggest picks the edge to ignore: the weakest broken edge, else the weakest edge.
func suggest(in Input, edges []Edge, broken []int) int {
	pick := func(idx []int) int {
		best, bestKey := -1, 0
		for _, i := range idx {
			if !suggestable(edges[i]) {
				continue
			}
			if k := giveKey(in, edges[i]); best < 0 || k < bestKey {
				best, bestKey = i, k
			}
		}
		return best
	}
	if b := pick(broken); b >= 0 {
		return b
	}
	all := make([]int, len(edges))
	for i := range all {
		all[i] = i
	}
	return pick(all)
}

// orderCycle orders the members of one cycle (ms ascending = current order): it keeps the
// strongest constraints that can hold together — pairs by their strongest constraint,
// strongest first, each kept unless it closes a loop with those already kept — and sorts by
// them, keeping the current order where they allow. It returns the order and the indices
// of the edges it breaks.
func orderCycle(in Input, ms []int, edges []Edge) (order []int, broken []int) {
	type pair struct {
		from, to, key int
		edges         []int
	}
	byPair := map[[2]int]*pair{}
	var pairs []*pair
	for i, e := range edges {
		k := [2]int{e.From, e.To}
		p := byPair[k]
		if p == nil {
			p = &pair{from: e.From, to: e.To, key: -1}
			byPair[k] = p
			pairs = append(pairs, p)
		}
		p.key = max(p.key, giveKey(in, e))
		p.edges = append(p.edges, i)
	}
	sort.SliceStable(pairs, func(a, b int) bool {
		if pairs[a].key != pairs[b].key {
			return pairs[a].key > pairs[b].key
		}
		if pairs[a].from != pairs[b].from {
			return pairs[a].from < pairs[b].from
		}
		return pairs[a].to < pairs[b].to
	})
	adj := map[int][]int{}
	reaches := func(from, to int) bool {
		seen := map[int]bool{from: true}
		stack := []int{from}
		for len(stack) > 0 {
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v == to {
				return true
			}
			for _, w := range adj[v] {
				if !seen[w] {
					seen[w] = true
					stack = append(stack, w)
				}
			}
		}
		return false
	}
	for _, p := range pairs {
		if reaches(p.to, p.from) {
			broken = append(broken, p.edges...)
			continue
		}
		adj[p.from] = append(adj[p.from], p.to)
	}
	sort.Ints(broken)
	indeg := map[int]int{}
	for _, ws := range adj {
		for _, w := range ws {
			indeg[w]++
		}
	}
	done := map[int]bool{}
	for len(order) < len(ms) {
		for _, m := range ms { // earliest ready member in the current order
			if !done[m] && indeg[m] == 0 {
				done[m] = true
				order = append(order, m)
				for _, w := range adj[m] {
					indeg[w]--
				}
				break
			}
		}
	}
	return order, broken
}

// Result of Sort.
type Result struct {
	Order   []int // node indices in the new order
	Cycles  []Cycle
	Edges   []Edge
	Ignored int       // edges removed by ignore rules
	Dropped []Dropped // constraints not used because they cannot be satisfied (04 §5.1)
	Anchors []Anchor  // loadTop / loadBottom requests (04 §5.5)
}

// Reasons a constraint is dropped.
const (
	// ReasonOfficialOrder: a declaration or community rule between two official packages
	// that contradicts the official order.
	ReasonOfficialOrder = "contradicts the official order of Core and the DLCs"
	// ReasonOfficialFirst: a rule that puts a mod before Core or a DLC while other
	// constraints keep the mod after official content (it would form a cycle with the
	// built-in order).
	ReasonOfficialFirst = "would put the mod before official content that other constraints keep ahead of it"
)

// Dropped is a constraint the sorter does not use, with the reason.
type Dropped struct {
	Edge
	Reason string
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

// PositionRef is the ignore reference of a community loadTop / loadBottom.
func PositionRef(pkg string, t Tier) string {
	f := "loadTop"
	if t == TierBottom {
		f = "loadBottom"
	}
	return CommunityRef(pkg, f, "")
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

// Sort computes the new order (04 §5.2).
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
	add := func(from, to int, c Constraint) {
		if from == to {
			return
		}
		if c.Ref != "" && ignore[c.Ref] {
			res.Ignored++
			return
		}
		e := Edge{From: from, To: to, C: c}
		if c.Kind != KindOfficial && c.Kind != KindUser {
			rf, rt := officialRank(in.Nodes[from].PackageID), officialRank(in.Nodes[to].PackageID)
			if rf >= 0 && rt >= 0 && rf > rt {
				res.Dropped = append(res.Dropped, Dropped{e, ReasonOfficialOrder})
				return
			}
		}
		res.Edges = append(res.Edges, e)
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

	// A rule that puts a mod before official content, in a cycle with the built-in order:
	// drop it and look again (the mod then simply follows what it needs).
	var comp []int
	var members [][]int
	var seen map[[2]int]bool
	for round := 0; ; round++ {
		comp, members, seen = components(n, res.Edges)
		drop := map[int]bool{}
		if round < 4 {
			cyclic := map[int]bool{} // components with a built-in edge inside
			for _, e := range res.Edges {
				if e.C.Kind == KindOfficial && comp[e.From] == comp[e.To] {
					cyclic[comp[e.From]] = true
				}
			}
			for k, e := range res.Edges {
				if cyclic[comp[e.From]] && comp[e.From] == comp[e.To] && beforeOfficial(in, e) {
					drop[k] = true
				}
			}
		}
		if len(drop) == 0 {
			break
		}
		kept := res.Edges[:0:0]
		for k, e := range res.Edges {
			if drop[k] {
				res.Dropped = append(res.Dropped, Dropped{e, ReasonOfficialFirst})
			} else {
				kept = append(kept, e)
			}
		}
		res.Edges = kept
	}
	nComp := len(members)
	for c, ms := range members {
		if len(ms) < 2 {
			continue
		}
		cy := Cycle{}
		for _, e := range res.Edges {
			if comp[e.From] == c && comp[e.To] == c {
				cy.Edges = append(cy.Edges, e)
			}
		}
		cy.Members, cy.Broken = orderCycle(in, ms, cy.Edges)
		members[c] = cy.Members
		cy.Suggest = suggest(in, cy.Edges, cy.Broken)
		res.Cycles = append(res.Cycles, cy)
	}
	first := make([]int, nComp) // earliest current position of each component
	for c, ms := range members {
		first[c] = slices.Min(ms)
	}
	sort.Slice(res.Cycles, func(a, b int) bool { return slices.Min(res.Cycles[a].Members) < slices.Min(res.Cycles[b].Members) })

	// loadTop / loadBottom (04 §5.5): a top mod pulls what it needs to the top, a bottom mod
	// pulls what needs it to the bottom. A component's tier breaks ties in the Kahn pick.
	tier := tiers(in, &res, byLin, ignore, n)
	ctier := make([]Tier, nComp)
	for c, ms := range members {
		top, bottom := false, false
		for _, m := range ms {
			top = top || tier[m] == TierTop
			bottom = bottom || tier[m] == TierBottom
		}
		switch {
		case top && !bottom:
			ctier[c] = TierTop
		case bottom && !top:
			ctier[c] = TierBottom
		}
	}

	// Kahn over components: pick the ready component attached to what was just emitted,
	// then the one in the earlier tier, then the one earliest in the current list.
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
	key := func(c int) int { return first[c] }
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
			case ctier[c] != ctier[pick]:
				if ctier[c] < ctier[pick] {
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

// Place finds where newly enabled mods go (04 §2.1): in.Nodes[:n] is the current list,
// in order, and in.Nodes[n:] the new mods in the order given. The current list keeps its
// order; each new mod goes to the latest position its constraints allow — the end, unless
// an enabled mod has to load after it (a library, a DLC), then just before the first of
// them. A top mod (§5.5) goes to the earliest position instead, an `attach` mod right
// after its target. When the list already breaks its constraints so that no position
// satisfies both sides, the new mod goes after what it needs. It returns node indices.
func Place(in Input, n int) []int {
	r := Sort(in)
	own := map[int]Tier{}
	for _, a := range r.Anchors {
		own[a.Node] = a.Tier
	}
	byLin := map[string][]int{}
	for i, nd := range in.Nodes {
		if nd.Lineage != "" {
			byLin[nd.Lineage] = append(byLin[nd.Lineage], i)
		}
	}
	follows := map[int][]int{} // attach: node → targets
	for _, rl := range in.Rules {
		if rl.Kind != RuleAttach {
			continue
		}
		for _, a := range byLin[rl.A] {
			follows[a] = append(follows[a], byLin[rl.B]...)
		}
	}
	seq := make([]int, n)
	for i := range seq {
		seq[i] = i
	}
	for a := n; a < len(in.Nodes); a++ {
		pos := make(map[int]int, len(seq))
		for p, v := range seq {
			pos[v] = p
		}
		lo, hi := 0, len(seq)
		for _, e := range r.Edges {
			if p, ok := pos[e.From]; ok && e.To == a {
				lo = max(lo, p+1)
			}
			if p, ok := pos[e.To]; ok && e.From == a {
				hi = min(hi, p)
			}
		}
		at := hi
		switch {
		case lo > hi:
			at = lo
		case own[a] == TierTop:
			at = lo
		}
		for _, b := range follows[a] {
			if p, ok := pos[b]; ok && p+1 >= lo && p+1 <= hi {
				at = p + 1
				break
			}
		}
		seq = slices.Insert(seq, at, a)
	}
	return seq
}

// tiers resolves loadTop / loadBottom into a tier per node and records the anchors in res.
// A user rule wins over the community rule for the same mod; official content keeps its
// own place.
func tiers(in Input, res *Result, byLin map[string][]int, ignore map[string]bool, n int) []Tier {
	own := make([]Tier, n)
	if p, ok := in.Community.(Positions); ok {
		for i, nd := range in.Nodes {
			if nd.PackageID == "" || officialRank(nd.PackageID) >= 0 {
				continue
			}
			if t := p.Position(nd.PackageID); t != TierNormal {
				ref := PositionRef(nd.PackageID, t)
				if !ignore[ref] {
					own[i] = t
					res.Anchors = append(res.Anchors, Anchor{Node: i, Tier: t, Kind: KindCommunity, Ref: ref})
				}
			}
		}
	}
	for _, r := range in.Rules {
		t := map[string]Tier{RuleTop: TierTop, RuleBottom: TierBottom}[r.Kind]
		if t == TierNormal {
			continue
		}
		for _, i := range byLin[r.A] {
			if officialRank(in.Nodes[i].PackageID) >= 0 {
				continue
			}
			own[i] = t
			res.Anchors = slices.DeleteFunc(res.Anchors, func(a Anchor) bool { return a.Node == i })
			res.Anchors = append(res.Anchors, Anchor{Node: i, Tier: t, Kind: KindUser, RuleID: r.ID})
		}
	}
	if len(res.Anchors) == 0 {
		return own
	}
	sort.Slice(res.Anchors, func(a, b int) bool { return res.Anchors[a].Node < res.Anchors[b].Node })
	fwd := make([][]int, n)
	back := make([][]int, n)
	for _, e := range res.Edges {
		fwd[e.From] = append(fwd[e.From], e.To)
		back[e.To] = append(back[e.To], e.From)
	}
	reach := func(start int, adj [][]int) []int {
		seen := map[int]bool{start: true}
		stack, out := []int{start}, []int(nil)
		for len(stack) > 0 {
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, w := range adj[v] {
				if !seen[w] {
					seen[w] = true
					out = append(out, w)
					stack = append(stack, w)
				}
			}
		}
		sort.Ints(out)
		return out
	}
	top, bottom := make([]bool, n), make([]bool, n)
	for k := range res.Anchors {
		a := &res.Anchors[k]
		adj, mark := back, top
		if a.Tier == TierBottom {
			adj, mark = fwd, bottom
		}
		mark[a.Node] = true
		for _, m := range reach(a.Node, adj) {
			if officialRank(in.Nodes[m].PackageID) >= 0 {
				continue
			}
			mark[m] = true
			if own[m] == TierNormal {
				a.Pulled = append(a.Pulled, m)
			}
		}
	}
	out := make([]Tier, n)
	for i := range out {
		switch {
		case own[i] != TierNormal:
			out[i] = own[i] // an anchor keeps its own end
		case top[i] && !bottom[i]:
			out[i] = TierTop
		case bottom[i] && !top[i]:
			out[i] = TierBottom
		}
	}
	for k := range res.Anchors {
		a := &res.Anchors[k]
		a.Conflict = (a.Tier == TierTop && bottom[a.Node]) || (a.Tier == TierBottom && top[a.Node])
	}
	return out
}

// beforeOfficial reports a non-built-in edge that puts a mod before Core or a DLC.
func beforeOfficial(in Input, e Edge) bool {
	switch e.C.Kind {
	case KindLoadBefore, KindForceBef, KindCommunity:
		return officialRank(in.Nodes[e.From].PackageID) < 0 && officialRank(in.Nodes[e.To].PackageID) >= 0
	}
	return false
}

// components returns the strongly connected component of every node, the members of each
// component in ascending node order, and the set of distinct edges.
func components(n int, edges []Edge) (comp []int, members [][]int, seen map[[2]int]bool) {
	seen = map[[2]int]bool{}
	adj := make([][]int, n)
	for _, e := range edges {
		k := [2]int{e.From, e.To}
		if !seen[k] {
			seen[k] = true
			adj[e.From] = append(adj[e.From], e.To)
		}
	}
	for i := range adj {
		sort.Ints(adj[i])
	}
	comp = tarjan(n, adj)
	nComp := 0
	for _, c := range comp {
		nComp = max(nComp, c+1)
	}
	members = make([][]int, nComp)
	for i, c := range comp {
		members[c] = append(members[c], i)
	}
	return comp, members, seen
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
