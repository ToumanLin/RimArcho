package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/machine"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestMemoryTUIBackgroundComparison(t *testing.T) {
	e, _ := fixture(t)
	e.App.MemoryProbe = func() machine.Memory {
		return machine.Memory{TotalRAM: 8 << 30, AvailableRAM: 1, SampledAt: time.Now(), Adapters: []machine.Adapter{{Name: "fixture", Dedicated: 1}}}
	}
	m := direct(t, e, 160, 40)
	cmd := m.startMemoryEstimate(true)
	if cmd == nil || !m.prob.memoryReading {
		t.Fatal("no background measurement")
	}
	if m.startMemoryEstimate(true) != nil {
		t.Fatal("duplicate measurement")
	}
	msg := cmd().(memoryMeasuredMsg)
	m.memoryMeasured(msg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	found := false
	for _, it := range m.prob.items {
		if it.status == view.StatusErr && it.counted {
			found = true
		}
	}
	if !found {
		t.Fatal("severe RAM comparison not rendered")
	}
	if m.needCount() != countedNeeds(m) {
		t.Fatal("count mismatch")
	}
}

func TestMemoryLaunchCancelApproveAndStale(t *testing.T) {
	e, _ := fixture(t)
	e.MustRun("sort")
	e.App.MemoryProbe = func() machine.Memory {
		return machine.Memory{TotalRAM: 8 << 30, AvailableRAM: 1, SampledAt: time.Now()}
	}
	if _, err := e.App.PlanLoadCost(context.Background(), 0, nil); err != nil {
		t.Fatal(err)
	}
	m := direct(t, e, 160, 40)
	m.tab = tabHome
	m.homeKey("L")
	if m.panel == nil || m.panel.choice != 0 || m.panel.choices[0] != T("cancel") {
		t.Fatal("red launch did not default to cancel")
	}
	if text := strings.Join(m.panelLines(30), "\n"); !strings.Contains(text, T("loading RAM needed")) || !strings.Contains(text, T("RAM available now")) {
		t.Fatal(text)
	}
	m.panelKey("enter")
	if len(e.Launched) != 0 {
		t.Fatal("Enter launched instead of cancelling")
	}
	m.listAction(view.ListLaunch)
	e.MustRun("disable", "pkg:tester.a")
	m.panelKey("down")
	m.panelKey("enter")
	if len(e.Launched) != 0 {
		t.Fatal("stale confirmation launched")
	}
	e.MustRun("enable", "pkg:tester.a")
	e.MustRun("sort")
	if _, err := e.App.PlanLoadCost(context.Background(), 0, nil); err != nil {
		t.Fatal(err)
	}
	m.listAction(view.ListLaunch)
	m.panelKey("down")
	m.panelKey("enter")
	if len(e.Launched) != 1 {
		t.Fatalf("approved launch count %d", len(e.Launched))
	}
}
