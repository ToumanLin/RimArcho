package main

import (
	"os"
	"path/filepath"
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
	if es := validate(&good, items, packets, nil); len(es) != 0 {
		t.Fatalf("good label rejected: %v", es)
	}
	bad := good
	bad.Gist = "other"
	bad.Causes = []Cause{{"c.mod", "source"}, {"nowhere.mod", "missing"}, {"a.mod", "culprit"}}
	bad.Stage = "loading"
	bad.Evidence = []string{"a paraphrase"}
	bad.Model = ""
	es := validate(&bad, items, packets, nil)
	for _, want := range []string{"gist", "c.mod", "nowhere.mod", "culprit", "stage", "verbatim", "model"} {
		if !strings.Contains(strings.Join(es, "\n"), want) {
			t.Errorf("no error mentions %q: %v", want, es)
		}
	}
	unknown := good
	unknown.Verdict, unknown.Causes = "cause", nil
	if es := validate(&unknown, items, packets, nil); len(es) == 0 {
		t.Error("cause without causes accepted")
	}
}

func TestValidateV2(t *testing.T) {
	dir := t.TempDir()
	cs := filepath.Join(dir, "Foo.cs")
	os.WriteFile(cs, []byte("namespace A\n{\npublic class Foo\n{\n\tpublic void Bar()\n\t{\n\t\tx.Do();\n\t}\n}\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "T01.log"), []byte("\n=== t g\n$ archo mod a.mod\nversion 1.2\n"), 0o644)
	items := map[string]Item{"T01": {Item: "T01", Gist: "g1", Key: "k1", Tier: 2}}
	packets := map[string]*Packet{"T01": {Item: "T01", Gist: "g1", Key: "k1", LogMods: []string{"a.mod", "b.mod"},
		LogVers: map[string]string{"a.mod": "1.0"}, Text: "## Message\nat A.Foo.Bar ()\nmissing.dep is NOT in the log's list"}}
	o := &vopts{ExecLogs: dir}
	good := Label{Schema: "corpuslabel/2", Item: "T01", Gist: "g1", Key: "k1", Verdict: "cause",
		Causes: []Cause{{"a.mod", "source"}}, Stage: "tick", CauseType: "version-mismatch",
		Fix: Fix{Kind: "update", Mods: []string{"a.mod"}}, Confidence: "high", Difficulty: "L3",
		Evidence: []string{"at A.Foo.Bar ()", "env: version 1.2"},
		Code:     []CodeQuote{{File: cs, Type: "A.Foo", Method: "Bar", Lines: "6-7", Text: "x.Do();"}},
		Versions: []VersionCheck{{Pkg: "a.mod", Log: "1.0", Installed: "1.2", Same: "no"}}, Model: "m", Prompt: "p"}
	if es := validate(&good, items, packets, o); len(es) != 0 {
		t.Fatalf("good v2 label rejected: %v", es)
	}
	bad := good
	bad.CauseType = "mod-bug" // fix kind update does not fit
	bad.Difficulty = "L4"
	bad.Evidence = []string{"env: not in the transcript"}
	bad.Code = []CodeQuote{{File: cs, Type: "A.Foo", Method: "Bar", Lines: "1-2", Text: "x.Do();"}, {File: cs, Type: "A.Nope", Lines: "7", Text: "y.Do();"}}
	bad.Versions = []VersionCheck{{Pkg: "a.mod", Log: "0.9", Same: "no"}}
	es := strings.Join(validate(&bad, items, packets, o), "\n")
	for _, want := range []string{"does not fit", "difficulty", "exec transcript", "not at lines", "type \"A.Nope\"", "not in the file", "log version"} {
		if !strings.Contains(es, want) {
			t.Errorf("no error mentions %q:\n%s", want, es)
		}
	}
	dep := good
	dep.CauseType, dep.Fix = "missing-dependency", Fix{Kind: "enable-dependency", Mods: []string{"missing.dep"}}
	if es := strings.Join(validate(&dep, items, packets, o), "\n"); !strings.Contains(es, "role missing") {
		t.Errorf("missing-dependency without a missing cause accepted: %s", es)
	}
	unk := good
	unk.Verdict, unk.Causes, unk.CauseType, unk.Fix, unk.Code = "unknown", nil, "unknown", Fix{Kind: "unknown"}, nil
	if es := validate(&unk, items, packets, o); len(es) != 0 {
		t.Errorf("unknown label rejected: %v", es)
	}
}

func TestBenchClass(t *testing.T) {
	for _, c := range []struct{ class, key, want string }{
		{"exception", "exception|Error in static constructor of X: System.TypeInitializationException: …", "static-ctor"},
		{"harmony-stack", "harmony-stack|System.TypeLoadException: Could not resolve type with token 01 from typeref", "assembly"},
		{"exception", "exception|System.NullReferenceException: Object reference not set|A.B.<.cctor>b__2_2", "exception"},
		{"xml-load", "xml-load|Exception loading def: System.TypeLoadException: …", "xml-load"},
		{"save-load", "save-load|…", ""},
	} {
		if got := benchClass(c.class, c.key); got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.class, c.key, got, c.want)
		}
	}
}

func TestDefNamedDropsSentencePeriod(t *testing.T) {
	m := defNamedRe.FindStringSubmatch("Failed to find Verse.ThingDef named konf. There are 10129 defs of this type loaded.")
	if m == nil || m[2] != "konf" {
		t.Fatalf("got %v", m)
	}
	m = defNamedRe.FindStringSubmatch("No Verse.SoundDef named Foo.Bar_1 found")
	if m == nil || m[2] != "Foo.Bar_1" {
		t.Fatalf("got %v", m)
	}
}
