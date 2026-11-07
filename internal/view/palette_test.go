package view

import (
	"strings"
	"testing"
)

// 16 §3.5: word starts in any order, camel-case words, and typos.
func TestPalScoreFuzzy(t *testing.T) {
	t.Parallel()
	e := func(name, alias, desc string) PalEntry { return PalEntry{Name: name, Alias: alias, Desc: desc} }
	for _, c := range []struct {
		q    string
		e    PalEntry
		want int
	}{
		{"add", e("Add Cube", "", ""), 6},
		{"cube", e("Add Cube", "", ""), 6}, // suffix
		{"d cu", e("Add Cube", "", ""), 5},
		{"ad cu", e("Add Cube", "", ""), 4},
		{"cu ad", e("Add Cube", "", ""), 4},                            // any order
		{"lib hug", e("HugsLib", "", ""), 4},                           // camel case
		{"ac", e("Add Cube", "", ""), 3},                               // subsequence
		{"add cueb", e("Add Cube", "", ""), 2},                         // a swap
		{"add cbu", e("Add Cube", "", ""), 0},                          // short words must be exact
		{"addd cube", e("Add Cube", "", ""), 2},                        // an extra letter
		{"sotr", e("x", "sort", ""), 2},                                // a swap in the alias
		{"subscirbe", e("x", "subscribe", ""), 2},                      // a swap in a long word
		{"vanila expnded", e("Vanilla Expanded Framework", "", ""), 3}, // letters in order
		{"vanilka expnded", e("Vanilla Expanded Framework", "", ""), 2},
		{"cu cu", e("Add Cube", "", ""), 0}, // each query word takes its own word
		{"git page", e("x", "", "+ Git repository download page"), 1},
		{"xyzw", e("Add Cube", "", ""), 0},
	} {
		if got := PalScore(c.q, c.e); got != c.want {
			t.Errorf("PalScore(%q, %q/%q) = %d, want %d", c.q, c.e.Name, c.e.Alias, got, c.want)
		}
	}
}

func TestPalRankFuzzy(t *testing.T) {
	t.Parallel()
	all := []PalEntry{
		{Kind: PalMod, ID: "a", Name: "Adaptive Storage"},
		{Kind: PalMod, ID: "b", Name: "Add Cube"},
		{Kind: PalMod, ID: "c", Name: "Alpha Animals"},
	}
	names := func(q string) string {
		var out []string
		for _, e := range PalRank(q, all) {
			out = append(out, e.Name)
		}
		return strings.Join(out, ", ")
	}
	if got := names("ad cu"); got != "Add Cube" {
		t.Errorf("ad cu: %s", got)
	}
	if got := names("addf cueb"); got != "Add Cube" {
		t.Errorf("addf cueb: %s", got)
	}
	// word starts outrank a subsequence, even a command's
	all = append(all, PalEntry{Kind: PalCommand, ID: "x", Name: "a dim cup"})
	if got := names("ad cu"); !strings.HasPrefix(got, "Add Cube") {
		t.Errorf("ad cu with a subsequence: %s", got)
	}
}

func TestPrefixDist(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		a, b string
		k    int
		want bool
	}{
		{"cube", "cube", 0, true},
		{"cub", "cube", 0, true},
		{"cueb", "cube", 1, true},
		{"cuxe", "cubes", 1, true},
		{"cbue", "cube", 0, false},
		{"xyzw", "cube", 1, false},
		{"stroage", "storage", 1, true},
		{"strogae", "storage", 1, false},
		{"strogae", "storage", 2, true},
	} {
		if got := prefixDist([]rune(c.a), []rune(c.b), c.k); got != c.want {
			t.Errorf("prefixDist(%q, %q, %d) = %v", c.a, c.b, c.k, got)
		}
	}
}

func BenchmarkPalRank(b *testing.B) {
	words := []string{"Vanilla", "Expanded", "Framework", "Hugs", "Lib", "Storage", "Animals", "Factions", "Weapons", "Psycasts"}
	var all []PalEntry
	for i := range 6000 {
		name := words[i%10] + " " + words[(i/10)%10] + words[(i/100)%10]
		all = append(all, PalEntry{Kind: PalMod, ID: name, Name: name, Desc: "author.package someone"})
	}
	b.ResetTimer()
	for range b.N {
		PalRank("vanila expnd", all)
	}
}
