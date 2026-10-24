package cmds_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadcost"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestPlanCostModsAndJSON(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.AddWorkshop("3101", testkit.Sample("tester.cost.a", "Alpha"))
	b := e.AddWorkshop("3102", testkit.Sample("tester.cost.b", "Beta"))
	e.WriteFile(filepath.Join(b, "Sounds", "clip.ogg"), strings.Repeat("x", 100))
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.cost.b", "tester.cost.a")
	e.MustRun("scan")
	plain := e.MustRun("plan", "cost")
	if strings.Contains(plain, "per-mod estimates") {
		t.Fatal("default output includes per-mod table")
	}
	text := e.MustRun("plan", "cost", "--mods")
	_, table, ok := strings.Cut(text, "per-mod estimates (mods only, load order):")
	if !ok || !regexp.MustCompile(`(?m)^\s+1\s+Beta\s+#\S+\s+.+–.+\s+.+–.+$`).MatchString(table) ||
		!regexp.MustCompile(`(?m)^\s+2\s+Alpha\s+#\S+\s+.+–.+\s+.+–.+$`).MatchString(table) {
		t.Fatalf("missing load-order table or byte ranges:\n%s", text)
	}
	e.MustRun("plan", "new", "cost-copy", "--from", "current")
	e.MustRun("disable", "pkg:tester.cost.b")
	for _, flags := range [][]string{{"--json"}, {"--mods", "--json"}} {
		args := append([]string{"plan", "cost", "cost-copy"}, flags...)
		output := e.MustRun(args...)
		var got app.LoadCost
		if err := json.Unmarshal([]byte(output), &got); err != nil {
			t.Fatalf("output must be JSON only: %v\n%s", err, output)
		}
		if got.Mods != 2 || got.Measured != 0 || got.Cached != 2 || len(got.PerMod) != 2 || got.PerMod[0].PackageID != "tester.cost.b" || got.PerMod[1].PackageID != "tester.cost.a" {
			t.Fatalf("selected plan's per-mod JSON: %+v", got)
		}
		if got.PerMod[0].Measure.Sounds[".ogg"].Bytes != 100 || got.PerMod[0].Ref == "" || got.PerMod[0].Name != "Beta" {
			t.Fatalf("raw measurements or identity missing: %+v", got.PerMod[0])
		}
		var raw []loadcost.Mod
		for _, m := range got.PerMod {
			if !reflect.DeepEqual(m.Est, loadcost.Estimate(got.Game, []loadcost.Mod{m.Measure})) {
				t.Fatalf("per-mod report: %+v", m)
			}
			raw = append(raw, m.Measure)
		}
		if !reflect.DeepEqual(got.Report, loadcost.Estimate(got.Game, raw)) {
			t.Fatal("JSON measurements do not reproduce totals")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(output), &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"ramLow", "ramHigh", "vramLow", "vramHigh", "parts", "game", "mods", "measured", "cached", "baseline", "top", "perMod"} {
			if _, ok := fields[key]; !ok {
				t.Fatalf("missing JSON field %q", key)
			}
		}
	}
	help := e.MustRun("plan", "cost", "--help")
	if !strings.Contains(help, "--mods") || !strings.Contains(help, "--json") {
		t.Fatalf("missing flag help:\n%s", help)
	}
	e.MustRun("plan", "new", "empty")
	var empty app.LoadCost
	if err := json.Unmarshal([]byte(e.MustRun("plan", "cost", "empty", "--json")), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.PerMod == nil || len(empty.PerMod) != 0 {
		t.Fatalf("empty per-mod output must be []: %+v", empty.PerMod)
	}
}
