package known

import (
	"regexp"
	"testing"
)

// The embedded dataset parses and keeps the privacy rules of design §8: aggregates from at
// least five players, no paths or user data in templates.
func TestEmbedded(t *testing.T) {
	d, err := Parse(knownJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Prereqs) == 0 || len(d.Combos) == 0 {
		t.Fatalf("empty dataset: %d prerequisites, %d combinations", len(d.Prereqs), len(d.Combos))
	}
	private := regexp.MustCompile(`(?i)users[\\/]|/home/|appdata|steamapps|[a-z]:[\\/]`)
	for _, p := range d.Prereqs {
		if p.Players < 5 || p.Logs < p.Players || p.Without < p.Logs || p.Mod == p.Needs || p.Mod == "" || p.Needs == "" {
			t.Errorf("prerequisite %+v", p)
		}
		if private.MatchString(p.Example) {
			t.Errorf("private example: %q", p.Example)
		}
		if len(d.PrereqsOf(p.Mod)) == 0 {
			t.Errorf("not indexed: %s", p.Mod)
		}
	}
	for _, c := range d.Combos {
		if c.Players < 5 || len(c.Templates) == 0 || c.Logs > c.With || len(c.Mods) == 0 || len(c.Mods) > 2 {
			t.Errorf("combination %+v", c)
		}
		for _, tp := range c.Templates {
			if private.MatchString(tp) {
				t.Errorf("private template: %q", tp)
			}
		}
		if c.Kind() != KindMod && c.RateWithout()*4 > c.Rate() {
			t.Errorf("not much worse than without: %+v", c)
		}
	}
}

func TestLookup(t *testing.T) {
	d, err := Parse([]byte(`{"prereqs":[{"mod":"A.B_steam","needs":"C","logs":5,"players":5,"without":10}],
"combos":[{"mods":["X","Y"],"templates":["t"],"logs":3,"with":4,"without":10,"withoutErr":1},{"mods":["X"],"version":"1.0","templates":["t"]},{"mods":["X"],"templates":["t"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if ps := d.PrereqsOf("a.b"); len(ps) != 1 || ps[0].Needs != "c" || ps[0].Rate() != 0.5 {
		t.Fatalf("prereqs %+v", ps)
	}
	cs := d.CombosOf("x")
	if len(cs) != 3 || cs[0].Kind() != KindPair || cs[1].Kind() != KindVersion || cs[2].Kind() != KindMod || cs[0].Rate() != 0.75 || cs[0].RateWithout() != 0.1 {
		t.Fatalf("combos %+v", cs)
	}
	g := &G5{SmallPairs: 7, SmallGone: 3, SamePairs: 9, Fixes: map[string]int{"disable-source": 3, "reorder": 1}}
	if s := g.Text(); s != "gone after a small list change 3/7, without a change 0/9 (disable-source 3, reorder 1)" {
		t.Fatal(s)
	}
}
