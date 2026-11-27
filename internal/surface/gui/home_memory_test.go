//go:build windows || cgo

package gui

import (
	"context"
	"github.com/ToumanLin/RimArcho/internal/machine"
	"testing"
	"time"
)

func TestHomeMemoryUsesCurrentListAndInvalidatesEdits(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	available, commit := int64(1), int64(0)
	e.App.MemoryProbe = func() machine.Memory {
		return machine.Memory{TotalRAM: 8 << 30, AvailableRAM: available, CommitAvailable: commit, SampledAt: time.Now()}
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
	// the commit has room; physical RAM below the expected working set warns (#31)
	commit, available = r.Capacity.RAMHigh, r.Capacity.WorkingSetHigh-1
	warning, err := api.HomeMemory()
	if err != nil || warning.Assessment.RAMSeverity != "warning" {
		t.Fatalf("%+v %v", warning, err)
	}
	h := api.Home()
	if h.Rows[0].Memory == nil || h.Rows[0].Memory.Token != warning.Token {
		t.Fatal("home omitted current comparison")
	}
	// The command uses the App the API's background work uses too: take the API's lock, as
	// the API's own calls do.
	api.mu.Lock()
	e.MustRun("disable", "pkg:tester.a")
	api.mu.Unlock()
	changed, err := api.HomeMemory()
	if err != nil || changed.Token == first.Token || changed.Assessment.Estimated {
		t.Fatalf("edit reused stale estimate: %+v %v", changed, err)
	}
}
