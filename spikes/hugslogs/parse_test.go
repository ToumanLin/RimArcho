package main

import (
	"reflect"
	"testing"
)

// The forms of "Loaded mods:" lines found in the raw corpus (counted over 400 headers: [mv:]
// 9,107, [v:] 2,558, [v:][mv:] 1,771, [ov:] 688, [ov:][mv:] 25, and lines without a marker).
func TestParseModLine(t *testing.T) {
	cases := []struct {
		line string
		want Mod
	}{
		{"Core(Ludeon.RimWorld): (no assemblies)", Mod{Name: "Core", Pkg: "ludeon.rimworld"}},
		{"Harmony(brrainz.harmony)[mv:2.3.4.0]: 0Harmony(2.3.6), HarmonyMod(2.3.4)",
			Mod{Name: "Harmony", Pkg: "brrainz.harmony", MV: "2.3.4.0", Assemblies: []string{"0Harmony(2.3.6)", "HarmonyMod(2.3.4)"}}},
		{"Combat Extended(CETeam.CombatExtended)[v:16.7.3.0]: CombatExtended(0.0.0)",
			Mod{Name: "Combat Extended", Pkg: "ceteam.combatextended", V: "16.7.3.0", Assemblies: []string{"CombatExtended(0.0.0)"}}},
		{"HugsLib(UnlimitedHugs.HugsLib)[ov:12.0.0]: 0Harmony(av:2.3.6,fv:1.2.0.1), HugsLib(av:1.0.0,fv:12.0.0)",
			Mod{Name: "HugsLib", Pkg: "unlimitedhugs.hugslib", OV: "12.0.0", Assemblies: []string{"0Harmony(av:2.3.6,fv:1.2.0.1)", "HugsLib(av:1.0.0,fv:12.0.0)"}}},
		{"Some Mod(a.b)[v:1.2][mv:1.2.0]: A(1.0)", Mod{Name: "Some Mod", Pkg: "a.b", V: "1.2", MV: "1.2.0", Assemblies: []string{"A(1.0)"}}},
		{"Other(c.d)[ov:5.0.0][mv:4.0]: (no assemblies)", Mod{Name: "Other", Pkg: "c.d", OV: "5.0.0", MV: "4.0"}},
		// brackets inside a version, a space in the package id, parentheses and ": " in the name
		{"Humanoid Alien Races zh-pack(RWZH.ChinesePack.HumanoidAlienRaces)[v:[1.6.1]2026.1.10]: (no assemblies)",
			Mod{Name: "Humanoid Alien Races zh-pack", Pkg: "rwzh.chinesepack.humanoidalienraces", V: "[1.6.1]2026.1.10"}},
		{"Toolmetrics Redux_zh 工具箱(Xavier0501.Toolmetrics Redux_zh): (no assemblies)",
			Mod{Name: "Toolmetrics Redux_zh 工具箱", Pkg: "xavier0501.toolmetrics redux_zh"}},
		{"Rimworld: Soundscape (1.6)(tro.soundscape)[mv:1.0]: Sound(1.0)",
			Mod{Name: "Rimworld: Soundscape (1.6)", Pkg: "tro.soundscape", MV: "1.0", Assemblies: []string{"Sound(1.0)"}}},
	}
	for _, c := range cases {
		got, ok := parseModLine(c.line)
		if !ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseModLine(%q) = %+v, %v; want %+v", c.line, got, ok, c.want)
		}
	}
	for _, bad := range []string{"", "Anomaly Research Asteroid_zh 异象小行星 简繁汉化(1.6)", "no package: here", "Empty()[mv:1]: x"} {
		if m, ok := parseModLine(bad); ok {
			t.Errorf("parseModLine(%q) = %+v, want no match", bad, m)
		}
	}
	if v := (Mod{MV: "1", V: "2", OV: "3"}).Version(); v != "3" {
		t.Errorf("Version = %q, want the override", v)
	}
}
