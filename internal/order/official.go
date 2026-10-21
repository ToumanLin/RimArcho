package order

import "sort"

// IsOfficial reports Core and the DLCs (also future ludeon.rimworld.* packages).
func IsOfficial(pkg string) bool { return officialRank(pkg) >= 0 }

// Move puts node Node right after node After.
type Move struct{ Node, After int }

// OfficialMoves is the cheapest way to put official content back in place (04 §4.4), from
// the built-in edges of a Sort result: either move the official packages that sit after
// mods back behind the official package before them (a DLC enabled later is appended to the
// end of the list: 23% of shared 1.6 logs), or move the few mods that sit ahead of official
// content down behind it, whichever takes fewer moves. nil when official content is in
// place, or when neither works in at most limit moves (then sort the list).
func OfficialMoves(in Input, edges []Edge, limit int) []Move {
	var officials []int
	for i, nd := range in.Nodes {
		if IsOfficial(nd.PackageID) {
			officials = append(officials, i)
		}
	}
	if len(officials) == 0 {
		return nil
	}
	// Mods that must follow official content (built-in official-first edges), and the
	// first of them in the list.
	follows := map[int]bool{}
	first, broken := len(in.Nodes), false
	for _, e := range edges {
		if e.C.Kind != KindOfficial {
			continue
		}
		if e.From > e.To {
			broken = true
		}
		if e.C.Field == "official-first" {
			follows[e.To] = true
			first = min(first, e.To)
		}
	}
	if !broken {
		return nil
	}
	ranked := append([]int(nil), officials...)
	sort.SliceStable(ranked, func(a, b int) bool {
		return officialRank(in.Nodes[ranked[a]].PackageID) < officialRank(in.Nodes[ranked[b]].PackageID)
	})

	// A: move the late official packages back. The ones in place must already be in
	// official order, and Core must be in place (there is nothing to put it after).
	var a []Move
	okA := true
	prevInPlace := -1
	for k, o := range ranked {
		if o < first {
			if o < prevInPlace {
				okA = false
			}
			prevInPlace = o
			continue
		}
		if k == 0 {
			okA = false
			break
		}
		a = append(a, Move{Node: o, After: ranked[k-1]})
	}
	// B: move the mods ahead of the last official package down behind it, keeping their order.
	last := ranked[len(ranked)-1]
	var b []Move
	okB := true
	for k := 1; k < len(ranked); k++ {
		if ranked[k] < ranked[k-1] {
			okB = false // the official packages themselves are out of order
		}
	}
	after := last
	for i := 0; i < last; i++ {
		if follows[i] {
			b = append(b, Move{Node: i, After: after})
			after = i
		}
	}
	switch {
	case okA && len(a) > 0 && len(a) <= limit && (!okB || len(a) <= len(b)):
		return a
	case okB && len(b) > 0 && len(b) <= limit:
		return b
	}
	return nil
}
