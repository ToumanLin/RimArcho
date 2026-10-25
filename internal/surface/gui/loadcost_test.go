package gui

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestPlanCostPageRequests(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for the list measurement regression tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/planmanage.mjs").CombinedOutput(); err != nil {
		t.Fatalf("list measurement page: %v\n%s", err, out)
	}
}

func TestPlanCostMeasurementCancelsOlderRequests(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	api.costMeasure.disk.Lock()
	locked := true
	defer func() {
		if locked {
			api.costMeasure.disk.Unlock()
		}
	}()
	start := func(id int64) *PlanCostPage {
		t.Helper()
		ch := make(chan *PlanCostPage, 1)
		go func() { p, _ := api.PlanLoadCost(id); ch <- p }()
		select {
		case p := <-ch:
			if p == nil || !p.Loading {
				t.Fatalf("pending measurement: %+v", p)
			}
			return p
		case <-time.After(5 * time.Second):
			t.Fatal("measurement blocked navigation")
			return nil
		}
	}
	old := start(0)
	next := start(0)
	api.publishLoadCost(context.Background(), old.Request, nil, "stale progress", nil)
	if api.PlanLoadCostMeasure(old.Request) != nil {
		t.Fatal("old request still visible")
	}
	if p := api.PlanLoadCostMeasure(next.Request); p.Progress != "" {
		t.Fatalf("stale update: %+v", p)
	}
	api.Leave("plans")
	if api.PlanLoadCostMeasure(next.Request) != nil {
		t.Fatal("leaving did not cancel measurement")
	}
	locked = false
	api.costMeasure.disk.Unlock()
	current := start(0)
	deadline := time.After(10 * time.Second)
	for {
		p := api.PlanLoadCostMeasure(current.Request)
		if p == nil {
			t.Fatal("current request disappeared")
		}
		if !p.Loading {
			if p.Error != "" || p.PlanCost == nil {
				t.Fatalf("measurement failed: %+v", p)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("measurement did not finish")
		case <-time.After(time.Millisecond):
		}
	}
}
