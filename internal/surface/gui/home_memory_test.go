package gui

import (
	"context"
	"github.com/ToumanLin/RimArcho/internal/machine"
	"testing"
	"time"
)

func TestHomeMemoryUsesCurrentListAndInvalidatesEdits(t *testing.T) {
	e, api, _ := fixture(t)
	available := int64(1)
	e.App.MemoryProbe = func() machine.Memory {
		return machine.Memory{TotalRAM: 8 << 30, AvailableRAM: available, SampledAt: time.Now()}
	}
	first, err := api.HomeMemory()
	if err != nil || first == nil || first.Assessment.Estimated {
		t.Fatalf("initial %+v %v", first, err)
	}
	r, err := e.App.PlanLoadCost(context.Background(), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	severe, err := api.HomeMemory()
	if err != nil || severe.Assessment.RAMSeverity != "severe" {
		t.Fatalf("%+v %v", severe, err)
	}
	available = (r.Capacity.RAMLow + r.Capacity.RAMHigh) / 2
	warning, err := api.HomeMemory()
	if err != nil || warning.Assessment.RAMSeverity != "warning" {
		t.Fatalf("%+v %v", warning, err)
	}
	h := api.Home()
	if h.Rows[0].Memory == nil || h.Rows[0].Memory.Token != warning.Token {
		t.Fatal("home omitted current comparison")
	}
	e.MustRun("disable", "pkg:tester.a")
	changed, err := api.HomeMemory()
	if err != nil || changed.Token == first.Token || changed.Assessment.Estimated {
		t.Fatalf("edit reused stale estimate: %+v %v", changed, err)
	}
}
