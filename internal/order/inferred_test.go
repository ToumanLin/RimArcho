package order

import (
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/scan"
)

// 08 §1.6: rules inferred from XML patches are the weakest constraints.

func inferredNodes(bDecl scan.Decl) []Node {
	return []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("a.patcher", scan.Decl{}),
		node("c.free", scan.Decl{}),
		node("b.owner", bDecl),
	}
}

func TestInferredPatcherAfterOwner(t *testing.T) {
	in := Input{Nodes: inferredNodes(scan.Decl{}), GameMM: "1.6",
		Inferred: []Inferred{{Patcher: "i-a.patcher", Owner: "i-b.owner", Patches: 12, DefTypes: []string{"ThingDef", "HediffDef"}}}}
	r := Sort(in)
	if got := pkgs(in, r); !slices.Equal(got, []string{"ludeon.rimworld", "c.free", "b.owner", "a.patcher"}) {
		t.Fatalf("order = %v", got)
	}
	var inf *Edge
	for i, e := range r.Edges {
		if e.C.Kind == KindInferred {
			inf = &r.Edges[i]
		}
	}
	if inf == nil || inf.C.Ref != "inferred:L-a.patcher:L-b.owner" {
		t.Fatalf("inferred edge = %+v", inf)
	}
	if why := in.InferredWhy(*inf); why != "patches its ThingDefs, HediffDefs (12 patches)" {
		t.Fatalf("why = %q", why)
	}
	// the current order breaks it: the "why" list explains the move
	if b := Broken(in, r); len(b) != 1 || b[0].C.Kind != KindInferred {
		t.Fatalf("broken = %+v", b)
	}
}

func TestInferredLosesToDeclaredRule(t *testing.T) {
	// b.owner declares it loads after a.patcher: the declaration wins, no cycle
	in := Input{Nodes: inferredNodes(scan.Decl{LoadAfter: []string{"a.patcher"}}), GameMM: "1.6",
		Inferred: []Inferred{{Patcher: "i-a.patcher", Owner: "i-b.owner", Patches: 3, DefTypes: []string{"ThingDef"}}}}
	r := Sort(in)
	if got := pkgs(in, r); !slices.Equal(got, []string{"ludeon.rimworld", "a.patcher", "c.free", "b.owner"}) {
		t.Fatalf("order = %v", got)
	}
	if len(r.Cycles) != 0 || len(r.Dropped) != 1 || r.Dropped[0].C.Kind != KindInferred || r.Dropped[0].Reason != ReasonInferredWeaker {
		t.Fatalf("cycles %+v dropped %+v", r.Cycles, r.Dropped)
	}
	// a user rule wins the same way
	in = Input{Nodes: inferredNodes(scan.Decl{}), GameMM: "1.6", Rules: []Rule{{ID: 1, Kind: RuleBefore, A: "L-a.patcher", B: "L-b.owner"}},
		Inferred: in.Inferred}
	if got := pkgs(in, Sort(in)); !slices.Equal(got, []string{"ludeon.rimworld", "a.patcher", "c.free", "b.owner"}) {
		t.Fatalf("with a user rule: %v", got)
	}
}

func TestInferredCycleDropsInferredFirst(t *testing.T) {
	// two Mods patch each other: the one with more patches holds, the other is not used,
	// and there is no order.cycle
	in := Input{Nodes: inferredNodes(scan.Decl{}), GameMM: "1.6", Inferred: []Inferred{
		{Patcher: "i-a.patcher", Owner: "i-b.owner", Patches: 2, DefTypes: []string{"ThingDef"}},
		{Patcher: "i-b.owner", Owner: "i-a.patcher", Patches: 5, DefTypes: []string{"ThingDef"}},
	}}
	r := Sort(in)
	if got := pkgs(in, r); !slices.Equal(got, []string{"ludeon.rimworld", "a.patcher", "c.free", "b.owner"}) {
		t.Fatalf("order = %v", got)
	}
	if len(r.Cycles) != 0 || len(r.Dropped) != 1 || r.Dropped[0].To != 1 {
		t.Fatalf("cycles %+v dropped %+v", r.Cycles, r.Dropped)
	}
	// a cycle among declared rules stays a cycle of declared rules; an inferred edge
	// inside it is not used
	nodes := []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("x.one", scan.Decl{LoadAfter: []string{"y.two"}}),
		node("y.two", scan.Decl{LoadAfter: []string{"x.one"}}),
	}
	in = Input{Nodes: nodes, GameMM: "1.6", Inferred: []Inferred{{Patcher: "i-x.one", Owner: "i-y.two", Patches: 9}}}
	r = Sort(in)
	if len(r.Cycles) != 1 || len(r.Dropped) != 1 || r.Dropped[0].C.Kind != KindInferred {
		t.Fatalf("cycles %+v dropped %+v", r.Cycles, r.Dropped)
	}
	for _, e := range r.Cycles[0].Edges {
		if e.C.Kind == KindInferred {
			t.Fatalf("inferred edge in the cycle: %+v", e)
		}
	}
}

func TestInferredIgnoreRule(t *testing.T) {
	inf := []Inferred{{Patcher: "i-a.patcher", Owner: "i-b.owner", Patches: 4}}
	in := Input{Nodes: inferredNodes(scan.Decl{}), GameMM: "1.6", Inferred: inf,
		Rules: []Rule{{ID: 1, Kind: RuleIgnore, Target: InferredRef("L-a.patcher", "L-b.owner")}}}
	r := Sort(in)
	if got := pkgs(in, r); !slices.Equal(got, []string{"ludeon.rimworld", "a.patcher", "c.free", "b.owner"}) || r.Ignored != 1 {
		t.Fatalf("order = %v, ignored %d", got, r.Ignored)
	}
	// official content is never an end of an inferred rule
	in = Input{Nodes: inferredNodes(scan.Decl{}), GameMM: "1.6", Inferred: []Inferred{{Patcher: "i-ludeon.rimworld", Owner: "i-b.owner", Patches: 1}}}
	if r := Sort(in); len(r.Edges) != 3 {
		t.Fatalf("edges = %+v", r.Edges)
	}
}
