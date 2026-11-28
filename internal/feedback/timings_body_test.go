package feedback

import (
	"encoding/json"
	"strings"
	"testing"
)

// The body: one field to a line, phases as one ordered object, each slow operation on
// one line; 1.1.5's phase array still reads back.
func TestTimingsBody(t *testing.T) {
	tm := Timings{Version: "1.1.6", Priority: "normal", WallMs: 75400, Phases: Phases{{"read", 33}, {"patches", 43800}, {"other", 900}},
		Patches: &TimingPatches{Ops: 7495, Slowest: []TimingOp{{Ms: 12100.5, Pkg: "a.b", WS: "123", Class: "PatchOperationReplace", XPath: "Defs/X", File: "Patches/A.xml"}}},
		Harmony: &TimingHarmony{DLLs: 3}}
	want := `{
  "version": "1.1.6",
  "os": "",
  "active_mods": 0,
  "total_mods": 0,
  "automatic": false,
  "priority": "normal",
  "wall_ms": 75400,
  "phases": {"read":33,"patches":43800,"other":900},
  "patches": {
    "ops": 7495,
    "nodes": 0,
    "too_slow": 0,
    "parse_ms": 0,
    "sim_ms": 0,
    "facts_ms": 0,
    "slowest": [
      {"ms":12100.5,"pkg":"a.b","ws":"123","class":"PatchOperationReplace","xpath":"Defs/X","file":"Patches/A.xml"}
    ]
  },
  "harmony": {
    "dlls": 3,
    "cached": 0,
    "patches": 0,
    "methods": 0
  }
}`
	if got := tm.Body(); got != want {
		t.Fatalf("body\n%s\nwant\n%s", got, want)
	}
	var back Timings
	if err := json.Unmarshal([]byte(tm.Body()), &back); err != nil || back.Phases.Ms("patches") != 43800 || back.Phases[2].Name != "other" {
		t.Fatal(err, back.Phases)
	}
	old := `{"wall_ms":1,"phases":[{"name":"read","ms":33},{"name":"other","ms":27200}]}`
	if err := json.Unmarshal([]byte(old), &back); err != nil || back.Phases.Ms("other") != 27200 {
		t.Fatal(err, back.Phases)
	}
}

// Nothing that looks like a path on this computer is sent; Mod-relative files are kept.
func TestTimingsScrub(t *testing.T) {
	home := `C:\Users\Someone`
	tm := Timings{Patches: &TimingPatches{Slowest: []TimingOp{
		{Pkg: "a.b", XPath: `Defs/ThingDef[defName="C:\Mods\x"]`, File: `C:\Games\Mods\1\Patches\A.xml`},
		{Pkg: "a.b", XPath: `Defs/X`, File: `Patches\Sub\B.xml`, Class: `/home/someone/x`},
		{Pkg: "a.b", XPath: `Defs/` + strings.Repeat("y", 500), File: `\\server\share\C.xml`},
		{Pkg: "a.b", XPath: `Defs/li[text()="someone"]`, File: `/opt/mods/E.xml`, Class: `X.someone.Op`},
	}}, Harmony: &TimingHarmony{Slowest: []TimingDLL{{Pkg: "a.b", File: `../../Users/Someone/D.dll`}}}}
	tm.Scrub(home)
	s := tm.Patches.Slowest
	if s[0].XPath != "" || s[0].File != "A.xml" || s[1].File != "Patches/Sub/B.xml" || s[1].XPath != "Defs/X" || s[1].Class != "" ||
		len([]rune(s[2].XPath)) != XPathMax || s[2].File != "C.xml" || s[3].File != "E.xml" || tm.Harmony.Slowest[0].File != "D.dll" {
		t.Fatalf("%+v %+v", s, tm.Harmony.Slowest)
	}
	if b := tm.Body(); strings.Contains(b, `:\\`) || strings.Contains(strings.ToLower(b), "users") {
		t.Fatalf("a path in %s", b)
	}
}
