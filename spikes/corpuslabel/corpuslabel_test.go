package main

import (
	"strings"
	"testing"
)

func TestRawModsKeepsVersionSuffixes(t *testing.T) {
	head := "Loaded mods:\n" +
		"Harmony(brrainz.harmony)[mv:2.3.4.0]: 0Harmony(2.3.6), HarmonyMod(2.3.4)\n" +
		"Combat Extended(CETeam.CombatExtended)[v:16.7.3.0]: CombatExtended(0.0.0), VEFCompat(0.0.0)\n" +
		"RunAndGun - Continued(MemeGoddess.RunAndGun)[ov:5.0.0]: RunAndGun(1.0.0)\n" +
		"Core(Ludeon.RimWorld): (no assemblies)\n" +
		"Active Harmony patches:\n" +
		"Foo(bar.baz): nothing\n"
	ms := rawMods(head)
	var got []string
	for _, m := range ms {
		got = append(got, m.Pkg+"@"+m.MV)
	}
	want := "brrainz.harmony@2.3.4.0 ceteam.combatextended@16.7.3.0 memegoddess.runandgun@5.0.0 ludeon.rimworld@"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %v", got)
	}
	if len(ms[1].Asm) != 2 || ms[1].Asm[1] != "VEFCompat(0.0.0)" || ms[3].Asm != nil {
		t.Fatalf("assemblies %v / %v", ms[1].Asm, ms[3].Asm)
	}
}

func TestValidate(t *testing.T) {
	items := map[string]Item{"P01": {Item: "P01", Gist: "g1", Key: "k1"}}
	packets := map[string]*Packet{"P01": {Item: "P01", Gist: "g1", Key: "k1", LogMods: []string{"a.mod", "b.mod"},
		Text: "## Message\nat AMod.Thing.Do ()\nmissing.dep is NOT in the log's list"}}
	good := Label{Schema: "corpuslabel/1", Item: "P01", Gist: "g1", Key: "k1", Verdict: "cause",
		Causes: []Cause{{"a.mod", "source"}, {"missing.dep", "missing"}}, Stage: "tick", CauseType: "mod-bug",
		Fix: Fix{Kind: "update", Mods: []string{"a.mod"}}, Confidence: "high",
		Evidence: []string{"at AMod.Thing.Do ()", "log: something only the raw log has"}, Model: "m", Prompt: "p"}
	if es := validate(&good, items, packets); len(es) != 0 {
		t.Fatalf("good label rejected: %v", es)
	}
	bad := good
	bad.Gist = "other"
	bad.Causes = []Cause{{"c.mod", "source"}, {"nowhere.mod", "missing"}, {"a.mod", "culprit"}}
	bad.Stage = "loading"
	bad.Evidence = []string{"a paraphrase"}
	bad.Model = ""
	es := validate(&bad, items, packets)
	for _, want := range []string{"gist", "c.mod", "nowhere.mod", "culprit", "stage", "verbatim", "model"} {
		if !strings.Contains(strings.Join(es, "\n"), want) {
			t.Errorf("no error mentions %q: %v", want, es)
		}
	}
	unknown := good
	unknown.Verdict, unknown.Causes = "cause", nil
	if es := validate(&unknown, items, packets); len(es) == 0 {
		t.Error("cause without causes accepted")
	}
}
