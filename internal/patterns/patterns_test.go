package patterns_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/patterns"
)

func TestData(t *testing.T) {
	d := patterns.Load()
	if len(d.Rules) != 46 || len(d.Mod) < 1000 || d.Logs != 6734 {
		t.Fatalf("rules %d, mod %d, logs %d", len(d.Rules), len(d.Mod), d.Logs)
	}
	sum := 0
	for _, p := range d.Rules {
		sum += p.NClusters
		if !p.IsGeneric() || p.Stage == "" || p.Summary == "" {
			t.Errorf("%s: incomplete rule", p.ID)
		}
	}
	// Every problem cluster of the corpus falls under one rule (rt.other is the catch-all).
	if sum != d.Clusters {
		t.Errorf("rule clusters %d, total %d", sum, d.Clusters)
	}
	last := d.Rules[len(d.Rules)-1]
	if !last.Fallback || last.Match != "." {
		t.Errorf("last rule %s should be the catch-all fallback", last.ID)
	}
	// Privacy (00 §8): mod patterns only from enough logs and players, never paths.
	for _, p := range d.Mod {
		if p.Logs < 10 || p.Sessions < 5 {
			t.Errorf("%s: %d logs, %d sessions", p.ID, p.Logs, p.Sessions)
		}
		if strings.Contains(strings.ToLower(p.Template), "users\\") || strings.Contains(p.Template, ":\\") || strings.Contains(p.Template, "/home/") {
			t.Errorf("%s: template with a path", p.ID)
		}
		if !strings.HasPrefix(p.ID, "m:"+p.Mod+":") || d.Get(p.Kind) == nil && p.Kind != "" {
			t.Errorf("%s: bad id or kind %q", p.ID, p.Kind)
		}
	}
}

// The matcher works on the templates internal/logs builds from a real log.
func TestMatchLog(t *testing.T) {
	text := strings.Join([]string{
		"Could not resolve cross-reference: No Verse.SoundDef named Shot_Laser found to give to Verse.VerbProperties Verse.VerbProperties(laser gun) (using undefined sound instead)",
		"Could not resolve cross-reference: No Verse.ThingDef named Gun_X found to give to Verse.ThingDefCountClass Verse.ThingDefCountClass",
		"Could not load reference to Verse.ThingDef named Gun_Old",
		"Could not resolve reference to object with loadID Thing_Gun_Old12345 of type Verse.Thing. Was it compressed away, destroyed, had no ID number, or not saved/loaded right? curParent=null curPathRelToParent=",
		"Config error in Gun_Y: verb 0 has no defaultProjectile",
		"Could not load Texture2D at 'Things/Item/Missing' in any active mod or in base resources.",
		"Could not find type named VFECore.ThingDefExtension from node <li Class=\"VFECore.ThingDefExtension\">",
		"Exception ticking Pawn1: System.NullReferenceException: Object reference not set to an instance of an object",
		"  at Verse.Pawn.Tick () [0x00000] in <filename unknown>:0 ",
		"System.NullReferenceException: Object reference not set to an instance of an object",
		"  at SomeMod.Thing.DoIt () [0x00000] in <filename unknown>:0 ",
		"",
	}, "\n")
	f, err := logs.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	d := patterns.Load()
	got := map[string]bool{}
	for _, c := range logs.Clusters(f) {
		m := d.MatchCluster(c.Class, c.Template, nil)
		id := "-"
		if m.Generic != nil {
			id = m.Generic.ID
		}
		got[id] = true
		t.Logf("%s  %s", id, c.Template)
	}
	for _, want := range []string{"g:xref.sound", "g:xref.def", "g:save.defref", "g:save.loadid", "g:xml.config-error", "g:asset.texture-missing", "g:xml.type-missing", "g:rt.tick", "-"} {
		if !got[want] {
			t.Errorf("no cluster matched %s", want)
		}
	}
}

func TestFallbackAndMod(t *testing.T) {
	d := patterns.Load()
	// A bare NullReferenceException only names the class: no pattern on the cluster.
	if g := d.Generic("exception", "exception|System.NullReferenceException: Object reference not set to an instance of an object"); g == nil || !g.Fallback {
		t.Fatalf("NRE: %v", g)
	}
	if m := d.MatchCluster("exception", "exception|System.NullReferenceException: Object reference not set to an instance of an object", nil); m.Generic != nil {
		t.Errorf("fallback shown: %s", m.Generic.ID)
	}
	// A mod-specific pattern shows only for its own source mod.
	p := d.Mod[0]
	if m := d.MatchCluster(p.Class, p.Template, []string{p.Mod}); m.Mod != p {
		t.Errorf("mod pattern %s not matched for its mod", p.ID)
	}
	if m := d.MatchCluster(p.Class, p.Template, []string{"someone.else"}); m.Mod != nil {
		t.Errorf("mod pattern %s matched for another mod", p.ID)
	}
	if m := d.MatchCluster(p.Class, p.Template, nil); m.Mod != nil {
		t.Errorf("mod pattern without a source")
	}
	if d.Get("pattern:"+p.ID) != p || d.Get("xref.def") != d.Get("g:xref.def") || d.Get("pattern:g:xref.def") == nil || d.Get("nope") != nil {
		t.Error("Get")
	}
	if len(d.OfMod(p.Mod)) == 0 {
		t.Error("OfMod")
	}
}

func TestLines(t *testing.T) {
	r := &patterns.Resolution{Pairs: map[string]int{"none": 269, "small": 143}, Gone: map[string]int{"none": 16, "small": 27}, Fixes: map[string]int{"enable": 14, "disable-other": 15}}
	if got, want := r.Line(), "after 1-3 list changes it went away in 19% of 143 repeat uploads (without any change: 6% of 269)"; got != want {
		t.Errorf("Line: %q", got)
	}
	if got := patterns.FixList(r.Fixes); got != "disable-other 15, enable 14" {
		t.Errorf("FixList: %q", got)
	}
	if (&patterns.Resolution{Pairs: map[string]int{"small": 3}}).Line() != "" || (*patterns.Resolution)(nil).Line() != "" {
		t.Error("too few pairs should say nothing")
	}
	for n, want := range map[int]string{0: "0", 999: "999", 6734: "6,734", 610910: "610,910", 1234567: "1,234,567"} {
		if got := patterns.Thousands(n); got != want {
			t.Errorf("Thousands(%d) = %s", n, got)
		}
	}
}
