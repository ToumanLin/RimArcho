package order

// Sort acceptance S1–S5 (07 §8) at the algorithm level; the CLI path is covered by the
// golden and app tests.

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/scan"
)

func node(pkg string, d scan.Decl) Node {
	d.PackageID = pkg
	return Node{Instance: "i-" + pkg, PackageID: pkg, Lineage: "L-" + pkg, Decl: &d}
}

func pkgs(in Input, r Result) []string {
	var out []string
	for _, i := range r.Order {
		out = append(out, in.Nodes[i].PackageID)
	}
	return out
}

func base() []Node {
	return []Node{
		node("brrainz.harmony", scan.Decl{LoadBefore: []string{"Ludeon.RimWorld"}}),
		node("ludeon.rimworld", scan.Decl{}),
		node("ludeon.rimworld.biotech", scan.Decl{}),
		node("ludeon.rimworld.royalty", scan.Decl{}),
		node("a.lib", scan.Decl{}),
		node("b.mod", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "a.lib"}}, LoadAfter: []string{"brrainz.harmony"}}),
		node("c.mod", scan.Decl{}),
	}
}

func TestS1SortedIsStable(t *testing.T) {
	in := Input{Nodes: base(), GameMM: "1.6"}
	first := pkgs(in, Sort(in))
	want := []string{"brrainz.harmony", "ludeon.rimworld", "ludeon.rimworld.royalty", "ludeon.rimworld.biotech", "a.lib", "b.mod", "c.mod"}
	if !slices.Equal(first, want) {
		t.Fatalf("first sort = %v", first)
	}
	// Re-sorting the sorted list changes nothing.
	var sorted []Node
	for _, p := range first {
		for _, nd := range in.Nodes {
			if nd.PackageID == p {
				sorted = append(sorted, nd)
			}
		}
	}
	in2 := Input{Nodes: sorted, GameMM: "1.6"}
	r := Sort(in2)
	if got := pkgs(in2, r); !slices.Equal(got, first) || len(r.Cycles) != 0 {
		t.Fatalf("second sort = %v", got)
	}
}

func TestS2CycleAndS3Ignore(t *testing.T) {
	nodes := []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("x.one", scan.Decl{LoadAfter: []string{"y.two"}}),
		node("z.free", scan.Decl{}),
		node("y.two", scan.Decl{LoadAfter: []string{"x.one"}}),
	}
	in := Input{Nodes: nodes, GameMM: "1.6"}
	r := Sort(in)
	if len(r.Cycles) != 1 || !slices.Equal(r.Cycles[0].Members, []int{1, 3}) {
		t.Fatalf("cycles = %+v", r.Cycles)
	}
	var fields []string
	for _, e := range r.Cycles[0].Edges {
		fields = append(fields, fmt.Sprintf("%s:%s->%s", in.Nodes[e.C.Owner].PackageID, e.C.Field, e.C.Target))
	}
	slices.Sort(fields)
	if !slices.Equal(fields, []string{"x.one:loadAfter->y.two", "y.two:loadAfter->x.one"}) {
		t.Fatalf("cycle edges = %v", fields)
	}
	// Suggested ignore: same kind on both edges, so the one against the current order
	// (x.one loadAfter y.two, while x.one is listed first).
	if sg := r.Cycles[0].Suggest; sg < 0 || in.Nodes[r.Cycles[0].Edges[sg].C.Owner].PackageID != "x.one" {
		t.Fatalf("suggest = %d", sg)
	}
	// Both edges are equally strong: the one the current order keeps stays, so the cycle
	// keeps its order; the rest still sorts.
	if got := pkgs(in, r); !slices.Equal(got, []string{"ludeon.rimworld", "x.one", "y.two", "z.free"}) && !slices.Equal(got, []string{"ludeon.rimworld", "x.one", "z.free", "y.two"}) {
		t.Fatalf("order = %v", got)
	}
	// S3: ignore y.two's declaration → x.one after y.two.
	in.Rules = []Rule{{Kind: RuleIgnore, Target: DeclRef("L-y.two", "loadAfter", "x.one")}}
	r = Sort(in)
	got := pkgs(in, r)
	if len(r.Cycles) != 0 || r.Ignored != 1 || slices.Index(got, "y.two") > slices.Index(got, "x.one") {
		t.Fatalf("with ignore: %v cycles %d ignored %d", got, len(r.Cycles), r.Ignored)
	}
}

// A cycle gives up its weakest constraint, not whatever the current order happens to break
// (08 §1.2): a dependency outranks a loadAfter even when the list has it the wrong way round.
func TestCycleBreaksWeakestEdge(t *testing.T) {
	nodes := []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("a.mod", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "b.mod"}}}),
		node("c.free", scan.Decl{}),
		node("b.mod", scan.Decl{LoadAfter: []string{"a.mod"}}),
	}
	in := Input{Nodes: nodes, GameMM: "1.6"}
	r := Sort(in)
	if len(r.Cycles) != 1 {
		t.Fatalf("cycles = %+v", r.Cycles)
	}
	cy := r.Cycles[0]
	if got := pkgs(in, r); !slices.Equal(got, []string{"ludeon.rimworld", "b.mod", "a.mod", "c.free"}) {
		t.Fatalf("order = %v", got)
	}
	if len(cy.Broken) != 1 || cy.Edges[cy.Broken[0]].C.Kind != KindLoadAfter {
		t.Fatalf("broken = %+v", cy.Broken)
	}
	if cy.Suggest != cy.Broken[0] {
		t.Fatalf("suggest %d, broken %v", cy.Suggest, cy.Broken)
	}
	// A three-mod loop with one weak edge: only that edge breaks, the rest hold.
	nodes = []Node{
		node("p.mod", scan.Decl{LoadAfter: []string{"r.mod"}}),
		node("q.mod", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "p.mod"}}}),
		node("r.mod", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "q.mod"}}}),
	}
	in = Input{Nodes: nodes}
	r = Sort(in)
	if got := pkgs(in, r); !slices.Equal(got, []string{"p.mod", "q.mod", "r.mod"}) || len(r.Cycles[0].Broken) != 1 {
		t.Fatalf("order = %v, broken %v", got, r.Cycles[0].Broken)
	}
}

type posCommunity map[string]Tier

func (p posCommunity) Rules(string) ([]string, []string) { return nil, nil }
func (p posCommunity) Position(pkg string) Tier          { return p[pkg] }

// loadTop / loadBottom (08 §1.5): a top mod goes up with what it needs, a bottom mod goes
// down with what needs it; official content stays first; a user rule wins; the community
// one can be ignored.
func TestTiers(t *testing.T) {
	nodes := []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("perf.mod", scan.Decl{}),
		node("needs.perf", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "perf.mod"}}}),
		node("a.mod", scan.Decl{}),
		node("lib.x", scan.Decl{}),
		node("z.mod", scan.Decl{}),
		node("top.mod", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "lib.x"}}}),
		node("ludeon.rimworld.royalty", scan.Decl{}),
	}
	comm := posCommunity{"perf.mod": TierBottom, "top.mod": TierTop}
	in := Input{Nodes: nodes, GameMM: "1.6", Community: comm}
	r := Sort(in)
	want := []string{"ludeon.rimworld", "ludeon.rimworld.royalty", "lib.x", "top.mod", "a.mod", "z.mod", "perf.mod", "needs.perf"}
	if got := pkgs(in, r); !slices.Equal(got, want) {
		t.Fatalf("order = %v", got)
	}
	if len(r.Anchors) != 2 || r.Anchors[1].Kind != KindCommunity || !slices.Equal(r.Anchors[1].Pulled, []int{4}) {
		t.Fatalf("anchors = %+v", r.Anchors)
	}
	// Sorting the sorted list again changes nothing.
	var sorted []Node
	for _, i := range r.Order {
		sorted = append(sorted, nodes[i])
	}
	in2 := Input{Nodes: sorted, GameMM: "1.6", Community: comm}
	if got := pkgs(in2, Sort(in2)); !slices.Equal(got, want) {
		t.Fatalf("re-sort = %v", got)
	}
	// A user rule wins over the community: top.mod to the bottom.
	in.Rules = []Rule{{ID: 7, Kind: RuleBottom, A: "L-top.mod"}}
	r = Sort(in)
	if got := pkgs(in, r); !slices.Equal(got, []string{"ludeon.rimworld", "ludeon.rimworld.royalty", "a.mod", "lib.x", "z.mod", "perf.mod", "needs.perf", "top.mod"}) {
		t.Fatalf("user bottom: %v", got)
	}
	// Ignoring the community loadBottom puts perf.mod back among the rest.
	in.Rules = []Rule{{Kind: RuleIgnore, Target: PositionRef("perf.mod", TierBottom)}}
	r = Sort(in)
	if got := pkgs(in, r); !slices.Equal(got, []string{"ludeon.rimworld", "ludeon.rimworld.royalty", "lib.x", "top.mod", "perf.mod", "needs.perf", "a.mod", "z.mod"}) {
		t.Fatalf("ignored bottom: %v", got)
	}
	// Pulled both ways: shared depends on a bottom mod and a top mod needs it. It stays
	// normal; the constraints still hold.
	nodes = []Node{
		node("bot.mod", scan.Decl{}),
		node("shared", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "bot.mod"}}}),
		node("free", scan.Decl{}),
		node("up.mod", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "shared"}}}),
	}
	in = Input{Nodes: nodes, Community: posCommunity{"bot.mod": TierBottom, "up.mod": TierTop}}
	r = Sort(in)
	if got := pkgs(in, r); !slices.Equal(got, []string{"free", "bot.mod", "shared", "up.mod"}) {
		t.Fatalf("conflict: %v", got)
	}
	if !r.Anchors[0].Conflict || !r.Anchors[1].Conflict {
		t.Fatalf("anchors = %+v", r.Anchors)
	}
}

// Enabling without a position (07 §2.1): after what it needs, else before what needs it,
// else the end.
func TestPlace(t *testing.T) {
	cur := []Node{
		node("brrainz.harmony", scan.Decl{LoadBefore: []string{"Ludeon.RimWorld"}}),
		node("ludeon.rimworld", scan.Decl{}),
		node("ludeon.rimworld.anomaly", scan.Decl{}),
		node("needs.lib", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "the.lib"}}}),
		node("other.mod", scan.Decl{}),
		node("orig.mod", scan.Decl{}),
	}
	place := func(rules []Rule, comm Community, add ...Node) []string {
		in := Input{Nodes: append(slices.Clone(cur), add...), GameMM: "1.6", Rules: rules, Community: comm}
		var out []string
		for _, i := range Place(in, len(cur)) {
			out = append(out, in.Nodes[i].PackageID)
		}
		return out
	}
	base := []string{"brrainz.harmony", "ludeon.rimworld", "ludeon.rimworld.anomaly", "needs.lib", "other.mod", "orig.mod"}
	with := func(at int, pkg ...string) []string { return slices.Insert(slices.Clone(base), at, pkg...) }
	cases := []struct {
		name  string
		rules []Rule
		comm  Community
		add   []Node
		want  []string
	}{
		{"unconstrained: end", nil, nil, []Node{node("new.mod", scan.Decl{})}, with(6, "new.mod")},
		{"needs Harmony: after the official content, not before Core", nil, nil, []Node{node("new.mod", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "brrainz.harmony"}}})}, with(3, "new.mod")},
		{"needs other.mod: right after it", nil, nil, []Node{node("new.mod", scan.Decl{LoadAfter: []string{"other.mod"}})}, with(5, "new.mod")},
		{"a DLC: after the official content", nil, nil, []Node{node("ludeon.rimworld.odyssey", scan.Decl{})}, with(3, "ludeon.rimworld.odyssey")},
		{"a library an enabled mod needs: just before it", nil, nil, []Node{node("the.lib", scan.Decl{})}, with(3, "the.lib")},
		{"attach: right after its target", []Rule{{Kind: RuleAttach, A: "L-orig.mod.zh", B: "L-other.mod"}}, nil, []Node{node("orig.mod.zh", scan.Decl{})}, with(5, "orig.mod.zh")},
		{"two new, the second needed by the first", nil, nil, []Node{
			node("new.a", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "new.b"}}}), node("new.b", scan.Decl{})}, with(6, "new.b", "new.a")},
	}
	for _, c := range cases {
		if got := place(c.rules, c.comm, c.add...); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v", c.name, got)
		}
	}
	// The list already has a dependent before its dependency: the new mod sits between
	// them in neither order, so it goes after what it needs.
	cur = []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("wants.mid", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "mid.mod"}}}),
		node("base.mod", scan.Decl{}),
	}
	if got := place(nil, nil, node("mid.mod", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "base.mod"}}})); !slices.Equal(got, []string{"ludeon.rimworld", "wants.mid", "base.mod", "mid.mod"}) {
		t.Errorf("inconsistent list: %v", got)
	}
}

type fakeCommunity map[string][]string

func (f fakeCommunity) Rules(pkg string) ([]string, []string) { return f[pkg], nil }

func TestCycleSuggestPrefersCommunity(t *testing.T) {
	nodes := []Node{
		node("a.mod", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "b.mod"}}}),
		node("b.mod", scan.Decl{}),
	}
	// Dependency: b before a (conflicts with current order); community: a before b.
	in := Input{Nodes: nodes, Community: fakeCommunity{"b.mod": {"a.mod"}}}
	r := Sort(in)
	if len(r.Cycles) != 1 {
		t.Fatalf("cycles = %+v", r.Cycles)
	}
	cy := r.Cycles[0]
	if cy.Suggest < 0 || cy.Edges[cy.Suggest].C.Kind != KindCommunity {
		t.Fatalf("suggest = %+v", cy)
	}
}

func TestS4Attach(t *testing.T) {
	nodes := []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("orig.mod", scan.Decl{}),
		node("other.one", scan.Decl{}),
		node("other.two", scan.Decl{}),
		node("orig.mod.zh", scan.Decl{}),
	}
	in := Input{Nodes: nodes, GameMM: "1.6", Rules: []Rule{{Kind: RuleAttach, A: "L-orig.mod.zh", B: "L-orig.mod"}}}
	got := pkgs(in, Sort(in))
	if !slices.Equal(got, []string{"ludeon.rimworld", "orig.mod", "orig.mod.zh", "other.one", "other.two"}) {
		t.Fatalf("attach: %v", got)
	}
}

func TestS5FiveHundred(t *testing.T) {
	nodes := []Node{node("ludeon.rimworld", scan.Decl{})}
	for i := 0; i < 499; i++ {
		d := scan.Decl{}
		if i > 0 {
			d.Dependencies = []scan.Dependency{{PackageID: fmt.Sprintf("m.%d", (i*7)%i)}}
			d.LoadAfter = []string{fmt.Sprintf("m.%d", i/2)}
		}
		if i%10 == 0 && i > 20 {
			d.LoadBefore = []string{fmt.Sprintf("m.%d", i-3)}
		}
		nodes = append(nodes, node(fmt.Sprintf("m.%d", i), d))
	}
	// Reverse the non-official part so the sort has real work to do.
	slices.Reverse(nodes[1:])
	in := Input{Nodes: nodes, GameMM: "1.6"}
	start := time.Now()
	r := Sort(in)
	el := time.Since(start)
	if el > time.Second {
		t.Fatalf("500 mods took %v", el)
	}
	pos := map[int]int{}
	for p, i := range r.Order {
		pos[i] = p
	}
	if len(r.Order) != 500 {
		t.Fatalf("order has %d nodes", len(r.Order))
	}
	for _, e := range r.Edges {
		inCycle := false
		for _, c := range r.Cycles {
			if slices.Contains(c.Members, e.From) && slices.Contains(c.Members, e.To) {
				inCycle = true
			}
		}
		if !inCycle && pos[e.From] > pos[e.To] {
			t.Fatalf("edge %v violated", e)
		}
	}
	t.Logf("500 mods, %d edges, %d cycles: %v", len(r.Edges), len(r.Cycles), el)
}
