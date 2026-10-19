package loadout

import "sort"

// Change kinds of a Loadout diff (04 §3).
const (
	Added    = "added"
	Removed  = "removed"
	Moved    = "moved"
	Replaced = "replaced" // an instance swapped for another of the same lineage
)

// Change is one difference between two lists. Positions are 1-based; 0 means "absent".
type Change struct {
	Kind  string
	Entry Entry // the entry in the new list (for removed: the old entry)
	Old   Entry // replaced: the entry it replaced
	From  int
	To    int
}

// Diff reports how b differs from a: additions, removals, instance replacements within a
// lineage, and moves. Moves are the common entries outside a longest common subsequence,
// so shifting one mod reports one move rather than every entry in between.
func Diff(a, b List, lineage func(instance string) string) []Change {
	posA, posB := map[string]int{}, map[string]int{}
	for i, e := range a {
		posA[e.Key()] = i + 1
	}
	for i, e := range b {
		posB[e.Key()] = i + 1
	}
	var removed, added []Entry
	for _, e := range a {
		if posB[e.Key()] == 0 {
			removed = append(removed, e)
		}
	}
	for _, e := range b {
		if posA[e.Key()] == 0 {
			added = append(added, e)
		}
	}
	var out []Change
	usedRemoved := map[int]bool{}
	for _, ad := range added {
		replaced := false
		if ad.Instance != "" && lineage != nil {
			if lin := lineage(ad.Instance); lin != "" {
				for i, rm := range removed {
					if !usedRemoved[i] && rm.Instance != "" && lineage(rm.Instance) == lin {
						usedRemoved[i] = true
						out = append(out, Change{Kind: Replaced, Entry: ad, Old: rm, From: posA[rm.Key()], To: posB[ad.Key()]})
						replaced = true
						break
					}
				}
			}
		}
		if !replaced {
			out = append(out, Change{Kind: Added, Entry: ad, To: posB[ad.Key()]})
		}
	}
	for i, rm := range removed {
		if !usedRemoved[i] {
			out = append(out, Change{Kind: Removed, Entry: rm, From: posA[rm.Key()]})
		}
	}
	// Common entries in a's order and in b's order; LCS keeps the stable ones.
	var ca, cb []string
	for _, e := range a {
		if posB[e.Key()] != 0 {
			ca = append(ca, e.Key())
		}
	}
	for _, e := range b {
		if posA[e.Key()] != 0 {
			cb = append(cb, e.Key())
		}
	}
	stable := lcs(ca, cb)
	for _, e := range b {
		k := e.Key()
		if posA[k] != 0 && !stable[k] {
			out = append(out, Change{Kind: Moved, Entry: e, From: posA[k], To: posB[k]})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := out[i].To, out[j].To
		if pi == 0 {
			pi = out[i].From
		}
		if pj == 0 {
			pj = out[j].From
		}
		return pi < pj
	})
	return out
}

func lcs(a, b []string) map[string]bool {
	n, m := len(a), len(b)
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	keep := map[string]bool{}
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i] == b[j]:
			keep[a[i]] = true
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return keep
}
