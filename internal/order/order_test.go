package order

// Sort acceptance S1–S5 (04 §4.3) at the algorithm level; the CLI path is covered by the
// golden and app tests.

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/rimarcho/rimarcho/internal/scan"
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
	// Cycle members keep their relative order; the rest still sorts.
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
