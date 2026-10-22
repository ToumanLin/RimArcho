package order

import (
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/scan"
)

type communityBoth map[string][2][]string

func (c communityBoth) Rules(pkg string) ([]string, []string) { return c[pkg][0], c[pkg][1] }

// A community rule between two DLCs against the official order is dropped, not a cycle.
func TestDropContradictsOfficialOrder(t *testing.T) {
	nodes := []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("ludeon.rimworld.royalty", scan.Decl{}),
		node("ludeon.rimworld.biotech", scan.Decl{}),
	}
	in := Input{Nodes: nodes, Community: communityBoth{"ludeon.rimworld.royalty": {[]string{"ludeon.rimworld.biotech"}, nil}}}
	r := Sort(in)
	if len(r.Cycles) != 0 || len(r.Dropped) != 1 || r.Dropped[0].Reason != ReasonOfficialOrder {
		t.Fatalf("cycles %+v dropped %+v", r.Cycles, r.Dropped)
	}
	if got := pkgs(in, r); !slices.Equal(got, []string{"ludeon.rimworld", "ludeon.rimworld.royalty", "ludeon.rimworld.biotech"}) {
		t.Fatalf("order %v", got)
	}
}

// A mod that declares loadBefore Core but depends on a mod that loads after official
// content: the loadBefore is dropped instead of reporting a cycle.
func TestDropBeforeOfficialInCycle(t *testing.T) {
	nodes := []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("ludeon.rimworld.odyssey", scan.Decl{}),
		node("unlimitedhugs.hugslib", scan.Decl{}),
		node("x.early", scan.Decl{LoadBefore: []string{"Ludeon.RimWorld"}, Dependencies: []scan.Dependency{{PackageID: "unlimitedhugs.hugslib"}}}),
	}
	in := Input{Nodes: nodes, GameMM: "1.6"}
	r := Sort(in)
	if len(r.Cycles) != 0 {
		t.Fatalf("cycles %+v", r.Cycles)
	}
	if len(r.Dropped) != 1 || r.Dropped[0].Reason != ReasonOfficialFirst || r.Dropped[0].C.Field != "loadBefore" {
		t.Fatalf("dropped %+v", r.Dropped)
	}
	if got := pkgs(in, r); !slices.Equal(got, []string{"ludeon.rimworld", "ludeon.rimworld.odyssey", "unlimitedhugs.hugslib", "x.early"}) {
		t.Fatalf("order %v", got)
	}
	// The same for a community rule that puts a mod before a DLC.
	nodes = []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("ludeon.rimworld.ideology", scan.Decl{}),
		node("sf.grim.reality", scan.Decl{}),
	}
	in = Input{Nodes: nodes, Community: communityBoth{"sf.grim.reality": {nil, []string{"ludeon.rimworld.ideology"}}}}
	r = Sort(in)
	if len(r.Cycles) != 0 || len(r.Dropped) != 1 || r.Dropped[0].Reason != ReasonOfficialFirst {
		t.Fatalf("community: cycles %+v dropped %+v", r.Cycles, r.Dropped)
	}
}

// Harmony-style loadBefore Core is satisfiable and still honoured.
func TestBeforeOfficialKeptWhenSatisfiable(t *testing.T) {
	in := Input{Nodes: base(), GameMM: "1.6"}
	r := Sort(in)
	if len(r.Dropped) != 0 || pkgs(in, r)[0] != "brrainz.harmony" {
		t.Fatalf("dropped %+v order %v", r.Dropped, pkgs(in, r))
	}
}

func TestAssess(t *testing.T) {
	nodes := []Node{
		node("ludeon.rimworld", scan.Decl{}),
		node("ludeon.rimworld.odyssey", scan.Decl{}),
		node("superniquito.modoptionssort", scan.Decl{LoadBefore: []string{"Ludeon.RimWorld"}}),
		node("a.lib", scan.Decl{}),
		node("b.mod", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "a.lib"}}, LoadAfter: []string{"c.mod"}, LoadBefore: []string{"d.mod"}}),
		node("c.mod", scan.Decl{}),
		node("d.mod", scan.Decl{}),
		node("e.mod", scan.Decl{LoadAfter: []string{"ludeon.rimworld.odyssey"}}),
		node("nals.facialanimation", scan.Decl{}),
		node("erdelf.humanoidalienraces", scan.Decl{}),
		node("vanillaexpanded.gravship", scan.Decl{}),
		node("lts.mgw", scan.Decl{Dependencies: []scan.Dependency{{PackageID: "vanillaexpanded.gravship"}}}),
	}
	in := Input{Nodes: nodes, GameMM: "1.6", Community: communityBoth{
		"nals.facialanimation": {nil, []string{"erdelf.humanoidalienraces"}},
		"c.mod":                {nil, []string{"d.mod"}},
	}}
	r := Sort(in)
	got := map[string]string{}
	for _, e := range r.Edges {
		if e.C.Kind == KindOfficial {
			continue
		}
		a := Assess(in, e)
		got[e.C.Kind+" "+in.Nodes[e.From].PackageID+">"+in.Nodes[e.To].PackageID] = a.Strength.String()
		if a.Strength == Harmless && !strings.Contains(a.Why, "corpus") {
			t.Errorf("harmless without a reason: %+v", a)
		}
	}
	want := map[string]string{
		"loadBefore superniquito.modoptionssort>ludeon.rimworld":   "harmless", // listed in observed.json
		"dependency a.lib>b.mod":                                   "strong",
		"loadAfter c.mod>b.mod":                                    "weak",
		"loadBefore b.mod>d.mod":                                   "strong",
		"loadAfter ludeon.rimworld.odyssey>e.mod":                  "strong",
		"community nals.facialanimation>erdelf.humanoidalienraces": "harmless",
		"community c.mod>d.mod":                                    "weak",
		"dependency vanillaexpanded.gravship>lts.mgw":              "strong", // observed harmful
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %q, want %q", k, got[k], w)
		}
	}
	// A loadBefore Core not in the table is harmless by kind.
	e := Edge{From: 3, To: 0, C: Constraint{Kind: KindLoadBefore}}
	if a := Assess(in, e); a.Strength != Harmless {
		t.Errorf("a.lib loadBefore Core = %v", a)
	}
}

func TestOfficialMoves(t *testing.T) {
	names := func(in Input, ms []Move) []string {
		var out []string
		for _, m := range ms {
			out = append(out, in.Nodes[m.Node].PackageID+">"+in.Nodes[m.After].PackageID)
		}
		return out
	}
	mk := func(pkgs ...string) Input {
		var nodes []Node
		for _, p := range pkgs {
			d := scan.Decl{}
			if p == "brrainz.harmony" {
				d.LoadBefore = []string{"ludeon.rimworld"}
			}
			nodes = append(nodes, node(p, d))
		}
		return Input{Nodes: nodes, GameMM: "1.6"}
	}
	// Odyssey enabled later: appended to the end.
	in := mk("brrainz.harmony", "ludeon.rimworld", "ludeon.rimworld.biotech", "a.mod", "b.mod", "c.mod", "ludeon.rimworld.odyssey")
	if got := names(in, OfficialMoves(in, Sort(in).Edges, 3)); !slices.Equal(got, []string{"ludeon.rimworld.odyssey>ludeon.rimworld.biotech"}) {
		t.Fatalf("odyssey late: %v", got)
	}
	// One mod above Core: move the mod, not six official packages.
	in = mk("a.mod", "ludeon.rimworld", "ludeon.rimworld.royalty", "ludeon.rimworld.biotech", "b.mod")
	if got := names(in, OfficialMoves(in, Sort(in).Edges, 3)); !slices.Equal(got, []string{"a.mod>ludeon.rimworld.biotech"}) {
		t.Fatalf("mod above core: %v", got)
	}
	// In place: nothing.
	in = mk("brrainz.harmony", "ludeon.rimworld", "ludeon.rimworld.odyssey", "a.mod")
	if got := OfficialMoves(in, Sort(in).Edges, 3); got != nil {
		t.Fatalf("in place: %v", got)
	}
	// Too many moves either way: sort instead.
	in = mk("a.mod", "b.mod", "ludeon.rimworld", "c.mod", "ludeon.rimworld.royalty", "ludeon.rimworld.ideology", "d.mod", "ludeon.rimworld.biotech", "ludeon.rimworld.odyssey")
	if got := OfficialMoves(in, Sort(in).Edges, 2); got != nil {
		t.Fatalf("too many: %v", got)
	}
}
