// Package order sorts a Loadout under hard constraints (08 §1): official order, dependencies,
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

// Rule kinds (08 §2.1).
const (
	RuleAfter        = "after"
	RuleBefore       = "before"
	RuleIncompatible = "incompatible"
	RuleAttach       = "attach"
	RuleIgnore       = "ignore"
	RuleTop          = "top"    // A loads as early as its constraints allow (08 §1.5)
	RuleBottom       = "bottom" // A loads as late as its constraints allow
	// RuleReplaces: A stands in for the package Target wherever a mod requires that package
	// and it is not enabled (08 §1.1, §2.1). Not a sorting rule by itself: the dependency
	// orders after A (Input.Replacements).
	RuleReplaces = "replaces"
)

// Rule is a user rule; A and B are lineage IDs, Target an ignore reference.
type Rule struct {
	ID     int64
	Kind   string
	A, B   string
	Target string
}

// Community supplies community ordering rules by packageId (08 §2.3).
type Community interface {
	Rules(pkg string) (loadAfter, loadBefore []string)
}

// Positions is implemented by a Community that also has loadTop / loadBottom (08 §1.5).
type Positions interface {
	Position(pkg string) Tier
}

// Tier is a preferred end of the list (08 §1.5). It only breaks ties between mods the
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
	// Replacements: for a required package no node has, the packageIds of nodes that replace
	// it (Use This Instead, 08 §1.1). The dependency then orders after those.
	Replacements map[string][]string
	// Observed: the log corpus's measured constraints for this game version (05 §4.6), from
	// the caller's corpus choice; nil uses the embedded 1.6 file.
	Observed ObservedSet
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

// kindRank orders constraint kinds by how readily a cycle gives them up (08 §1.2): community
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
	Dropped []Dropped // constraints not used because they cannot be satisfied (08 §1.1)
	Anchors []Anchor  // loadTop / loadBottom requests (08 §1.5)
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
	Reason  string
	Members []int // full component for a cycle with the built-in official order
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

// Sort computes the new order (08 §1.2).
func Sort(in Input) Result {
	s := newSorter(in)
	s.officialEdges()
	for i, nd := range in.Nodes {
		if nd.Decl != nil {
			s.declEdges(i, nd)
		}
	}
	if in.Community != nil {
		s.communityEdges()
	}
	attach := s.userEdges()
	comp, members, seen := s.dropOfficialFirst()
	nComp := len(members)
	s.cycles(comp, members)
	first := make([]int, nComp) // earliest current position of each component
	for c, ms := range members {
		first[c] = slices.Min(ms)
	}
	sort.Slice(s.res.Cycles, func(a, b int) bool {
		return slices.Min(s.res.Cycles[a].Members) < slices.Min(s.res.Cycles[b].Members)
	})

	// loadTop / loadBottom (08 §1.5): a top mod pulls what it needs to the top, a bottom mod
	// pulls what needs it to the bottom. A component's tier breaks ties in the Kahn pick.
	tier := tiers(in, &s.res, s.byLin, s.ignore, s.n)
	ctier := componentTiers(members, tier)
	s.res.Order = kahn(componentGraph{comp: comp, members: members, seen: seen, tier: ctier, first: first}, attach)
	return s.res
}

// sorter collects the constraints of one Sort as edges between nodes.
type sorter struct {
	in     Input
	n      int
	byPkg  map[string][]int
	byLin  map[string][]int
	ignore map[string]bool // ignore rules' references
	res    Result
}

// newSorter indexes the nodes by package id and lineage, and the ignore rules.
func newSorter(in Input) *sorter {
	s := &sorter{in: in, n: len(in.Nodes), byPkg: map[string][]int{}, byLin: map[string][]int{}, ignore: map[string]bool{}}
	for i, nd := range in.Nodes {
		if nd.PackageID != "" {
			s.byPkg[nd.PackageID] = append(s.byPkg[nd.PackageID], i)
		}
		if nd.Lineage != "" {
			s.byLin[nd.Lineage] = append(s.byLin[nd.Lineage], i)
		}
	}
	for _, r := range in.Rules {
		if r.Kind == RuleIgnore {
			s.ignore[r.Target] = true
		}
	}
	return s
}

// add keeps an edge from → to unless an ignore rule names it; a non-official constraint
// that contradicts the official order is dropped.
func (s *sorter) add(from, to int, c Constraint) {
	if from == to {
		return
	}
	if c.Ref != "" && s.ignore[c.Ref] {
		s.res.Ignored++
		return
	}
	e := Edge{From: from, To: to, C: c}
	if c.Kind != KindOfficial && c.Kind != KindUser {
		rf, rt := officialRank(s.in.Nodes[from].PackageID), officialRank(s.in.Nodes[to].PackageID)
		if rf >= 0 && rt >= 0 && rf > rt {
			s.res.Dropped = append(s.res.Dropped, Dropped{Edge: e, Reason: ReasonOfficialOrder})
			return
		}
	}
	s.res.Edges = append(s.res.Edges, e)
}

// pkgsOf is a list of package ids lower-cased and trimmed.
func pkgsOf(l []string) []string {
	out := make([]string, 0, len(l))
	for _, p := range l {
		out = append(out, strings.ToLower(strings.TrimSpace(p)))
	}
	return out
}

// officialEdges is the built-in order: official content first, in official order.
func (s *sorter) officialEdges() {
	in := s.in
	// Mods that explicitly load before an official package (Harmony) are exempt from the
	// built-in "official first" order; otherwise every such list would be a cycle.
	exempt := make([]bool, s.n)
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
		s.add(officials[k-1], officials[k], Constraint{Kind: KindOfficial, Owner: officials[k], Field: "official-order", Target: in.Nodes[officials[k-1]].PackageID, Ref: OfficialRef(o.Lineage)})
	}
	if len(officials) > 0 {
		last := officials[len(officials)-1]
		for i, nd := range in.Nodes {
			if officialRank(nd.PackageID) >= 0 || exempt[i] {
				continue
			}
			s.add(last, i, Constraint{Kind: KindOfficial, Owner: i, Field: "official-first", Target: in.Nodes[last].PackageID, Ref: OfficialRef(nd.Lineage)})
		}
	}
}

// declEdges are node i's About.xml declarations: its dependencies, then its load rules.
func (s *sorter) declEdges(i int, nd Node) {
	d := nd.Decl
	s.depEdges(i, nd)
	for _, f := range []struct {
		kind, field string
		list        []string
		before      bool
	}{
		{KindLoadBefore, "loadBefore", withVersion(d.LoadBefore, d.LoadBeforeByVersion, s.in.GameMM), true},
		{KindLoadAfter, "loadAfter", withVersion(d.LoadAfter, d.LoadAfterByVersion, s.in.GameMM), false},
		{KindForceBef, "forceLoadBefore", d.ForceLoadBefore, true},
		{KindForceAft, "forceLoadAfter", d.ForceLoadAfter, false},
	} {
		for _, p := range pkgsOf(f.list) {
			for _, j := range s.byPkg[p] {
				c := Constraint{Kind: f.kind, Owner: i, Field: f.field, Target: p, Ref: DeclRef(nd.Lineage, f.field, p)}
				if f.before {
					s.add(i, j, c)
				} else {
					s.add(j, i, c)
				}
			}
		}
	}
}

// depEdges are node i's dependencies, each on the package, its first alternative some
// node has, or its replacement.
func (s *sorter) depEdges(i int, nd Node) {
	d := nd.Decl
	deps := map[string][]string{} // required package → its alternativePackageIds
	for _, x := range append(slices.Clone(d.Dependencies), d.DependenciesByVersion["v"+s.in.GameMM]...) {
		p := strings.ToLower(x.PackageID)
		deps[p] = append(deps[p], pkgsOf(x.Alternatives)...)
	}
	depList := make([]string, 0, len(deps))
	for p := range deps {
		depList = append(depList, p)
	}
	sort.Strings(depList)
	for _, p := range depList {
		// The package itself; when no node has it, the first alternative some node has
		// (the game's ModDependency.IsSatisfied), then its replacements. The reference
		// stays the declared package, so one ignore rule covers every stand-in.
		targets, target := s.byPkg[p], p
		for _, alt := range deps[p] {
			if len(targets) > 0 {
				break
			}
			targets, target = s.byPkg[alt], alt
		}
		for _, r := range s.in.Replacements[p] {
			if len(targets) > 0 {
				break
			}
			targets, target = s.byPkg[r], r
		}
		for _, j := range targets {
			s.add(j, i, Constraint{Kind: KindDependency, Owner: i, Field: "modDependencies", Target: target, Ref: DeclRef(nd.Lineage, "dependency", p)})
		}
	}
}

// communityEdges are the community rules.
func (s *sorter) communityEdges() {
	for i, nd := range s.in.Nodes {
		if nd.PackageID == "" {
			continue
		}
		after, before := s.in.Community.Rules(nd.PackageID)
		for _, p := range pkgsOf(after) {
			for _, j := range s.byPkg[p] {
				s.add(j, i, Constraint{Kind: KindCommunity, Owner: i, Field: "loadAfter", Target: p, Ref: CommunityRef(nd.PackageID, "loadAfter", p)})
			}
		}
		for _, p := range pkgsOf(before) {
			for _, j := range s.byPkg[p] {
				s.add(i, j, Constraint{Kind: KindCommunity, Owner: i, Field: "loadBefore", Target: p, Ref: CommunityRef(nd.PackageID, "loadBefore", p)})
			}
		}
	}
}

// userEdges are the user rules, hard, between lineages; it returns by target node the
// nodes that want to follow it immediately.
func (s *sorter) userEdges() map[int][]int {
	attach := map[int][]int{}
	for _, r := range s.in.Rules {
		for _, a := range s.byLin[r.A] {
			for _, b := range s.byLin[r.B] {
				c := Constraint{Kind: KindUser, Owner: a, Field: r.Kind, Target: s.in.Nodes[b].PackageID, RuleID: r.ID}
				switch r.Kind {
				case RuleAfter:
					s.add(b, a, c)
				case RuleBefore:
					s.add(a, b, c)
				case RuleAttach:
					s.add(b, a, c)
					attach[b] = append(attach[b], a)
				}
			}
		}
	}
	return attach
}

// dropOfficialFirst drops each rule that puts a mod before official content in a cycle
// with the built-in order, and looks again (the mod then simply follows what it needs);
// it returns the components of what is left.
func (s *sorter) dropOfficialFirst() (comp []int, members [][]int, seen map[[2]int]bool) {
	res := &s.res
	for round := 0; ; round++ {
		comp, members, seen = components(s.n, res.Edges)
		drop := map[int]bool{}
		if round < 4 {
			cyclic := map[int]bool{} // components with a built-in edge inside
			for _, e := range res.Edges {
				if e.C.Kind == KindOfficial && comp[e.From] == comp[e.To] {
					cyclic[comp[e.From]] = true
				}
			}
			for k, e := range res.Edges {
				if cyclic[comp[e.From]] && comp[e.From] == comp[e.To] && beforeOfficial(s.in, e) {
					drop[k] = true
				}
			}
		}
		if len(drop) == 0 {
			return comp, members, seen
		}
		kept := res.Edges[:0:0]
		for k, e := range res.Edges {
			if drop[k] {
				res.Dropped = append(res.Dropped, Dropped{Edge: e, Reason: ReasonOfficialFirst, Members: append([]int(nil), members[comp[e.From]]...)})
			} else {
				kept = append(kept, e)
			}
		}
		res.Edges = kept
	}
}

// cycles orders each component of two or more nodes and records it as a cycle; members
// takes each cycle's order.
func (s *sorter) cycles(comp []int, members [][]int) {
	for c, ms := range members {
		if len(ms) < 2 {
			continue
		}
		cy := Cycle{}
		for _, e := range s.res.Edges {
			if comp[e.From] == c && comp[e.To] == c {
				cy.Edges = append(cy.Edges, e)
			}
		}
		cy.Members, cy.Broken = orderCycle(s.in, ms, cy.Edges)
		members[c] = cy.Members
		cy.Suggest = suggest(s.in, cy.Edges, cy.Broken)
		s.res.Cycles = append(s.res.Cycles, cy)
	}
}

// componentTiers is each component's tier: top or bottom when its members ask only for that.
func componentTiers(members [][]int, tier []Tier) []Tier {
	ctier := make([]Tier, len(members))
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
	return ctier
}

// componentGraph is the constraint graph between components.
type componentGraph struct {
	comp    []int
	members [][]int
	seen    map[[2]int]bool // the node edges
	tier    []Tier
	first   []int // earliest current position of each component
}

// kahn orders the components: it picks the ready component attached to what was just
// emitted, then the one in the earlier tier, then the one earliest in the current list.
func kahn(g componentGraph, attach map[int][]int) []int {
	nComp := len(g.members)
	indeg := make([]int, nComp)
	cadj := make([]map[int]bool, nComp)
	for i := range cadj {
		cadj[i] = map[int]bool{}
	}
	for k := range g.seen {
		a, b := g.comp[k[0]], g.comp[k[1]]
		if a != b && !cadj[a][b] {
			cadj[a][b] = true
			indeg[b]++
		}
	}
	ready := map[int]bool{}
	for c := 0; c < nComp; c++ {
		if indeg[c] == 0 {
			ready[c] = true
		}
	}
	var order []int
	boost := map[int]bool{}
	for len(ready) > 0 {
		pick := g.pick(ready, boost)
		delete(ready, pick)
		boost = map[int]bool{}
		order = append(order, g.members[pick]...)
		for _, m := range g.members[pick] {
			for _, a := range attach[m] {
				boost[g.comp[a]] = true
			}
		}
		for d := range cadj[pick] {
			indeg[d]--
			if indeg[d] == 0 {
				ready[d] = true
			}
		}
	}
	return order
}

// pick is the ready component to emit next: boosted, then the earlier tier, then the
// earliest in the current list.
func (g componentGraph) pick(ready, boost map[int]bool) int {
	pick := -1
	for c := range ready {
		switch {
		case pick < 0:
			pick = c
		case boost[c] != boost[pick]:
			if boost[c] {
				pick = c
			}
		case g.tier[c] != g.tier[pick]:
			if g.tier[c] < g.tier[pick] {
				pick = c
			}
		case g.first[c] < g.first[pick]:
			pick = c
		}
	}
	return pick
}

// Place finds where newly enabled mods go (07 §2.1): in.Nodes[:n] is the current list,
// in order, and in.Nodes[n:] the new mods in the order given. The current list keeps its
// order; each new mod goes right after the last enabled mod it must load after (never
// ahead of the official content); with only mods that must load after it (a library, a
// DLC), just before the first of them; with no constraint, at the end. An `attach` mod
// goes right after its target. The same rule as the editor's suggested position
// (app.SuggestAt); in the log-corpus replay it lands closer to where players had a mod
// than the latest legal position. It returns node indices.
func Place(in Input, n int) []int {
	r := Sort(in)
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
		lo, hi, official := 0, len(seq), 0
		after, before := false, false
		for _, e := range r.Edges {
			if p, ok := pos[e.From]; ok && e.To == a {
				if e.C.Kind == KindOfficial && e.C.Field == "official-first" {
					official = max(official, p+1) // a bound, not a reason to move up
					continue
				}
				lo, after = max(lo, p+1), true
			}
			if p, ok := pos[e.To]; ok && e.From == a {
				hi, before = min(hi, p), true
			}
		}
		at := len(seq)
		switch {
		case after:
			at = max(lo, official)
		case before:
			at = hi
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
