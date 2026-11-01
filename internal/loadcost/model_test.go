package loadcost

import (
	"encoding/json"
	"reflect"
	"testing"
)

func modelMod() Mod {
	return Mod{
		PNG: Files{3, 111}, PNGPixelsMip: 1000, PNGRaw16: MipFiles{1, 100},
		PNGNot4: 200, PNGSmall512Pixels: 300,
		DDS: Files{1, 400}, JPG: Files{1, 20}, PSD: Files{1, 10},
		Sounds: map[string]Sound{
			".wav": {Files{3, 1000}, Files{2, 300}, 400, Files{2, 600}},
			".ogg": {Files{2, 100}, Files{1, 20}, 220, Files{1, 20}},
		},
		Defs: Files{2, 100}, Patches: Files{1, 11}, Assemblies: Files{1, 50},
		Languages: Files{100, 1000000}, // Informational: absent from the estimate.
	}
}

func TestEstimate(t *testing.T) {
	for _, tc := range []struct {
		game Game
		want Report
	}{
		{Game16, Report{2022989, 2759828, 2160, 2780, []Part{
			{"textures", 5, 141, 2816, 4284, 1760, 2380},
			{"dds", 1, 400, 640, 720, 400, 400},
			{"sounds", 5, 1100, 131692, 132312, 0, 0},
			{"defs", 2, 100, 300, 800, 0, 0},
			{"patches", 1, 11, 5, 22, 0, 0},
			{"assemblies", 1, 50, 100, 250, 0, 0},
			{"mod overhead", 1, 0, 1887436, 2621440, 0, 0},
		}}},
		{Game17, Report{3815, 9632, 1674, 5668, []Part{
			{"textures", 5, 141, 2190, 6230, 1314, 4628},
			{"dds", 1, 400, 600, 1400, 360, 1040},
			{"sounds", 5, 1100, 620, 930, 0, 0},
			{"defs", 2, 100, 300, 800, 0, 0},
			{"patches", 1, 11, 5, 22, 0, 0},
			{"assemblies", 1, 50, 100, 250, 0, 0},
		}}},
	} {
		t.Run(map[Game]string{Game16: "1.6", Game17: "1.7"}[tc.game], func(t *testing.T) {
			got := Estimate(tc.game, []Mod{modelMod()})
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Estimate:\n got %+v\nwant %+v", got, tc.want)
			}
			var merged Mod
			merged.Plus(modelMod())
			merged.Plus(modelMod())
			separate, combined := Estimate(tc.game, []Mod{modelMod(), modelMod()}), Estimate(tc.game, []Mod{merged})
			if tc.game == Game16 {
				if !reflect.DeepEqual(separate.Parts[:6], combined.Parts[:6]) || separate.Parts[6].Count != 2 || combined.Parts[6].Count != 1 {
					t.Fatal("resource aggregation or per-Mod count changed")
				}
			} else if !reflect.DeepEqual(separate, combined) {
				t.Fatalf("separate and combined estimates differ: %+v, %+v", separate, combined)
			}
			empty := Estimate(tc.game, nil)
			if empty.RAMLow != 0 || empty.RAMHigh != 0 || empty.VRAMLow != 0 || empty.VRAMHigh != 0 || len(empty.Parts) != 6+map[Game]int{Game16: 1}[tc.game] {
				t.Fatalf("empty estimate: %+v", empty)
			}
		})
	}
}

func TestCalibrationAndRounding(t *testing.T) {
	saved := Coef
	t.Cleanup(func() { Coef = saved })
	Coef.Tex17VRAMLow, Coef.Tex17VRAMHigh = 0.5, 1
	Coef.Tex17RAMLow, Coef.Tex17RAMHigh = 0, 0.25
	r := Estimate(Game17, []Mod{{DDS: Files{1, 3}}})
	if r.VRAMLow != 1 || r.VRAMHigh != 3 || r.RAMHigh != 1 {
		t.Fatalf("calibrated/outward-rounded estimate: %+v", r)
	}
	// Rounding follows aggregation: two 3-byte DDSs have a 3-byte lower bound.
	r = Estimate(Game17, []Mod{{DDS: Files{1, 3}}, {DDS: Files{1, 3}}})
	if r.VRAMLow != 3 || r.RAMHigh != 2 {
		t.Fatalf("aggregate rounding: %+v", r)
	}
}

func TestModPlusDoesNotAliasSounds(t *testing.T) {
	source := modelMod()
	var sum Mod
	sum.Plus(source)
	sum.Plus(source)
	if source.Sounds[".wav"].Count != 3 || sum.Sounds[".wav"].Count != 6 {
		t.Fatalf("Plus changed source: %+v", source.Sounds)
	}
}

func TestReportJSON(t *testing.T) {
	r := Estimate(Game17, []Mod{modelMod()})
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 5 {
		t.Fatalf("unexpected JSON keys: %s", b)
	}
	for _, key := range []string{"ramLow", "ramHigh", "vramLow", "vramHigh", "parts"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("missing %q in %s", key, b)
		}
	}
}
