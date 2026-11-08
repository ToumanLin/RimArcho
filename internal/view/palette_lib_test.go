package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// libraryPalette builds the palette as the GUI and TUI do (view.CommandEntries, then every
// Mod with its package and author as the description) over a library of n Mods.
func libraryPalette(n int) []view.PalEntry {
	all := view.CommandEntries()
	for i, name := range testkit.ModNames(n) {
		all = append(all, view.PalEntry{Kind: view.PalMod, ID: name, Name: name, Desc: "author" + string(rune('a'+i%26)) + ".mod someone"})
	}
	return all
}

// palTop is the first k results: a command's ID, or a Mod's name.
func palTop(q string, all []view.PalEntry, k int) []string {
	var out []string
	for _, e := range view.PalRank(q, all) {
		if len(out) == k {
			break
		}
		if e.Kind == view.PalCommand {
			out = append(out, "cmd:"+e.ID)
		} else {
			out = append(out, e.Name)
		}
	}
	return out
}

// 16 §3.5 over a thousand Mods: what a player types finds what they mean, in English and
// Chinese, before the Mods whose long names merely hold the letters in order.
func TestPalRankLibrary(t *testing.T) {
	view.SetLang("en")
	defer view.SetLang("en")
	check := func(lang string) {
		all := libraryPalette(1000)
		loose := 0
		for _, e := range all {
			if e.Kind == view.PalMod && view.Subsequence("sotr", strings.ToLower(e.Name)) {
				loose++
			}
		}
		if loose < 100 {
			t.Fatalf("only %d Mods hold s-o-t-r in order; the library does not test the ranking", loose)
		}
		for _, c := range []struct {
			q     string
			first string
			also  string // in the first five
		}{
			{"sort", "cmd:sort", "cmd:preview-sort"},
			{"sotr", "cmd:sort", "cmd:preview-sort"},
			{"subscribe", "cmd:subscribe", ""},
			{"subscirbe", "cmd:subscribe", ""},
			{"sbscribe", "cmd:subscribe", ""},
			{"analyze", "cmd:analyze", "Multi Analyzer Retexture"},
			{"rescna", "cmd:rescan", ""},
			{"search files", "cmd:search-files", ""},
			{"hug lib", "HugsLib", "Log Publisher from HugsLib"},
			{"lib hug", "HugsLib", "Log Publisher from HugsLib"},
			{"ad cu", "Add Cube", ""},
			{"vfe", "Vanilla Factions Expanded - Mechanoids", ""},
		} {
			top := palTop(c.q, all, 5)
			if len(top) == 0 || top[0] != c.first || (c.also != "" && !strings.Contains(strings.Join(top, "|"), c.also)) {
				t.Errorf("%s %q: %v, want %s first", lang, c.q, top, c.first)
			}
		}
		// typos in two words: Vanilla … Expanded Mods first
		for _, name := range palTop("vanilka expnded", all, 10) {
			if !strings.HasPrefix(name, "Vanilla ") || !strings.Contains(name, "Expanded") {
				t.Errorf("%s vanilka expnded: %q among the first ten", lang, name)
			}
		}
	}
	check("en")
	view.SetLang("zh")
	check("zh")
	all := libraryPalette(1000)
	for _, c := range []struct{ q, first, also string }{
		{"排序", "cmd:sort", "cmd:rules"}, // suffix: automatic sorting and sorting rules
		{"订阅", "cmd:subscribe", ""},
		{"预览", "cmd:preview-sort", "cmd:preview-launch"},
		{"检查文件", "cmd:analyze", ""},
	} {
		top := palTop(c.q, all, 5)
		if len(top) == 0 || top[0] != c.first || (c.also != "" && !strings.Contains(strings.Join(top, "|"), c.also)) {
			t.Errorf("zh %q: %v, want %s first", c.q, top, c.first)
		}
	}
}

// BenchmarkPalRankLibrary ranks the commands and 6,000 Mods for typed queries that reach
// the typo and letters-in-order checks on most names (16 §3.5).
func BenchmarkPalRankLibrary(b *testing.B) {
	all := libraryPalette(6000)
	qs := []string{"sotr", "subscirbe", "vanilka expnded", "hug lib"}
	b.ResetTimer()
	for i := range b.N {
		view.PalRank(qs[i%len(qs)], all)
	}
}
