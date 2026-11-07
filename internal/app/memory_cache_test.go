package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/machine"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestMemoryEstimateCacheTracksPlanAndScan(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	modDir := e.AddWorkshop("3101", testkit.Sample("tester.memory", "Memory"))
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.memory")
	e.MustRun("scan")
	hardware := machine.Memory{TotalRAM: 16 << 30, AvailableRAM: 1 << 30, SampledAt: time.Now(), Adapters: []machine.Adapter{{Name: "fixture", Dedicated: 1 << 30}}}
	e.App.MemoryProbe = func() machine.Memory { return hardware }
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if e.App.CachedMemoryAssessment(st).Estimated {
		t.Fatal("unmeasured list has estimate")
	}
	cost, err := e.App.PlanLoadCost(context.Background(), st.Plan.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := e.App.CachedMemoryAssessment(st)
	if !c.Estimated || c.RAMSeverity != "severe" || cost.Capacity.RAMSeverity != c.RAMSeverity {
		t.Fatalf("missing severe assessment: %+v", c)
	}
	// Hardware changes reuse file totals, not a stale capacity verdict.
	hardware.AvailableRAM = 16 << 30
	if e.App.CachedMemoryAssessment(st).RAMSeverity != "ok" {
		t.Fatal("capacity verdict was cached with estimate")
	}
	reader, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if reader.MemorySnapshot().AvailableRAM != hardware.AvailableRAM {
		t.Fatal("test probe did not cross reader boundary")
	}
	e.MustRun("disable", "pkg:tester.memory")
	changed, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if e.App.CachedMemoryAssessment(changed).Estimated {
		t.Fatal("changed enabled list reused old totals")
	}
	e.MustRun("enable", "pkg:tester.memory")
	e.WriteFile(filepath.Join(modDir, "Defs", "changed.xml"), "<Defs><ThingDef><defName>New</defName></ThingDef></Defs>")
	e.MustRun("scan")
	rescanned, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if e.App.CachedMemoryAssessment(rescanned).Estimated {
		t.Fatal("changed scan snapshot reused old totals")
	}
	_, err = e.App.PlanLoadCost(context.Background(), rescanned.Plan.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := "memory-estimate:" + rescanned.Ix.Env.ID + ":" + fmt.Sprint(rescanned.Plan.ID)
	var saved map[string]any
	if err := json.Unmarshal([]byte(e.App.Store.Meta(key)), &saved); err != nil {
		t.Fatal(err)
	}
	saved["at"] = time.Now().Add(-61 * time.Minute)
	data, _ := json.Marshal(saved)
	if err := e.App.Store.SetMeta(key, string(data)); err != nil {
		t.Fatal(err)
	}
	// An old estimate still counts (no flicker back to unknown) but asks for a re-measure.
	if c := e.App.CachedMemoryAssessment(rescanned); !c.Estimated || !c.Stale {
		t.Fatalf("old estimate: estimated %v, stale %v", c.Estimated, c.Stale)
	}

}
