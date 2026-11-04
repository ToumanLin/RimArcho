package mcp_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/machine"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestMemoryToolParityAndSeverity(t *testing.T) {
	t.Parallel()
	got, err := mcp.ArgvFor("estimate_memory", []byte(`{"plan":"small"}`))
	if err != nil || !reflect.DeepEqual(got, []string{"plan", "cost", "small"}) {
		t.Fatalf("argv %v %v", got, err)
	}
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	available := int64(1)
	e.App.MemoryProbe = func() machine.Memory {
		return machine.Memory{TotalRAM: 8 << 30, AvailableRAM: available, SampledAt: time.Unix(100, 0), Adapters: []machine.Adapter{{Name: "synthetic", Dedicated: 1}}}
	}
	e.MustRun("plan", "cost") // warm the header cache for byte-identical counts
	cli := e.MustRun("plan", "cost")
	cs := connect(t, e)
	output, isErr := call(t, cs, "estimate_memory", nil)
	if isErr || output != cli {
		t.Fatalf("MCP/CLI differ:\n%s\n%s", output, cli)
	}
	if !strings.Contains(output, "[severe]") || !strings.Contains(output, "[warning]") {
		t.Fatalf("severity missing: %s", output)
	}
}

func TestMemoryToolRAMLevels(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	available, total := int64(1), int64(16<<30)
	e.App.MemoryProbe = func() machine.Memory {
		return machine.Memory{TotalRAM: total, AvailableRAM: available, SampledAt: time.Now()}
	}
	r, err := e.App.PlanLoadCost(context.Background(), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs := connect(t, e)
	for _, tc := range []struct {
		available int64
		want      string
	}{
		{r.Capacity.RAMLow - 1, "severe"}, {r.Capacity.RAMLow, "warning"}, {r.Capacity.RAMHigh, "ok"},
	} {
		available = tc.available
		cli := e.MustRun("plan", "cost")
		got, isErr := call(t, cs, "estimate_memory", nil)
		if isErr || got != cli {
			t.Fatal("CLI/MCP capacity mismatch")
		}
		line := ""
		for _, s := range strings.Split(got, "\n") {
			if strings.Contains(s, "RAM, available now:") {
				line = s
				break
			}
		}
		if !strings.Contains(line, "["+tc.want+"]") {
			t.Fatalf("%s: %s", tc.want, line)
		}
	}
	total = 0
	got, _ := call(t, cs, "estimate_memory", nil)
	if !strings.Contains(got, "unknown [unknown]") {
		t.Fatal(got)
	}
}
