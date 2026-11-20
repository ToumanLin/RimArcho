package view

import (
	"strings"
	"testing"
)

// 16 §3.5: word starts in any order, camel-case words, and typos.
func TestPalScoreFuzzy(t *testing.T) {
	t.Parallel()
	e := func(name, alias, desc string, match ...string) PalEntry {
		return PalEntry{Name: name, Alias: alias, Desc: desc, Match: strings.Join(match, "")}
	}
	for _, c := range []struct {
		q    string
		e    PalEntry
		want int
	}{
		{"add", e("Add Cube", "", ""), 6},
		{"cube", e("Add Cube", "", ""), 6}, // suffix
		{"d cu", e("Add Cube", "", ""), 5},
		{"ad cu", e("Add Cube", "", ""), 4},
		{"cu ad", e("Add Cube", "", ""), 4},                             // any order
		{"lib hug", e("HugsLib", "", ""), 4},                            // camel case
		{"ac", e("Add Cube", "", ""), 4},                                // initials
		{"vfe", e("Vanilla Factions Expanded - Mechanoids", "", ""), 4}, // initials of neighbouring words
		{"vem", e("Vanilla Factions Expanded - Mechanoids", "", ""), 2}, // not neighbours: letters in order
		{"add cueb", e("Add Cube", "", ""), 3},                          // a swap
		{"add cbu", e("Add Cube", "", ""), 0},                           // short words must be exact
		{"addd cube", e("Add Cube", "", ""), 3},                         // an extra letter
		{"sotr", e("x", "sort", ""), 3},                                 // a swap in the alias
		{"subscirbe", e("x", "subscribe", ""), 3},                       // a swap in a long word
		{"subscirbe", e("add a subscription", "", ""), 0},               // 3 edits from subscription
		{"subscirbe", e("x", "", "", "subscribe"), 3},                   // the match name (a command's ID)
		{"search files", e("x", "", "", "search files"), 6},
		{"srt", e("x", "sort", ""), 2},                                 // letters in order
		{"sotr", e("Echoes of the Rim", "", ""), 2},                    // letters in order across words
		{"vanila expnded", e("Vanilla Expanded Framework", "", ""), 3}, // typos at word starts
		{"vanilka expnded", e("Vanilla Expanded Framework", "", ""), 3},
		{"vnl xpnd", e("Vanilla Expanded Framework", "", ""), 2},
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

// 16 §3.5: every page command is a command, and no page lists one twice.
func TestPageCommandsExist(t *testing.T) {
	ids := map[string]bool{}
	for _, c := range Commands {
		ids[c.ID] = true
	}
	for page, list := range pageCommands {
		seen := map[string]bool{}
		for _, id := range list {
			if !ids[id] || seen[id] {
				t.Errorf("%s: %q unknown or listed twice", page, id)
			}
			seen[id] = true
		}
	}
}

// 16 §3.5: on a page, the empty query suggests its own commands in table order, then its
// parts, then the rest; a typed query matches everything, the page's own first among equals.
func TestPalRankPage(t *testing.T) {
	all := append(CommandEntries(), PalEntry{Kind: PalPart, ID: "0", Name: "interface"}, PalEntry{Kind: PalMod, ID: "m", Name: "Recheck Mod"})
	got := PalRankPage("", "needs", all)
	own := PageCommands("needs")
	for i, id := range own {
		if got[i].ID != id || !got[i].Own {
			t.Fatalf("needs, empty: #%d is %q, want %q", i, got[i].ID, id)
		}
	}
	if p := got[len(own)]; p.Kind != PalPart || !p.Own {
		t.Fatalf("needs, empty: after the commands %+v, want the part", p)
	}
	if r := got[len(own)+1]; r.Own || r.Kind != PalCommand {
		t.Fatalf("needs, empty: then %+v, want the other commands", r)
	}
	for _, e := range got {
		if e.Kind == PalMod {
			t.Fatal("empty query lists a Mod")
		}
	}
	// equal scores: the page's own first, in table order
	if g := PalRankPage("pre", EditorPage, all); g[0].ID != "preview-sort" {
		t.Fatalf("editor, pre: first %q", g[0].ID)
	}
	if g := PalRankPage("re", "needs", all); g[0].ID != "rescan" {
		t.Fatalf("needs, re: first %q, want the page's rescan before redo", g[0].ID)
	}
	if g := PalRankPage("re", EditorPage, all); g[0].ID != "redo" {
		t.Fatalf("editor, re: first %q, want the page's redo", g[0].ID)
	}
	if g := PalRankPage("recheck", "needs", all); len(g) == 0 || g[0].Kind != PalMod {
		t.Fatalf("typed queries still find Mods: %+v", g)
	}
	// without a page the order is PalRank's
	if g := PalRank("", all); g[0].ID != Commands[0].ID || g[0].Own {
		t.Fatalf("no page: first %+v", g[0])
	}
}
