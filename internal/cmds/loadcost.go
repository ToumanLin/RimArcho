package cmds

import (
	"encoding/json"
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	register(&Command{Name: "plan cost", Args: "[<plan>]", Summary: "estimate resident/loading RAM and VRAM, compare with hardware capacity", MaxArgs: 1,
		Flags: []Flag{
			{Name: "mods", Help: "include per-mod RAM and VRAM ranges in load order"},
			{Name: "json", Help: "print the full report and per-mod raw measurements as JSON"},
		}, Run: planCost})
}

func planCost(c *Ctx, args *Args) (*render.Doc, error) {
	p, err := c.App.CurrentPlan()
	if err != nil {
		return nil, err
	}
	if len(args.Pos) > 0 {
		p, err = c.App.FindPlan(args.Pos[0])
		if err != nil {
			return nil, err
		}
	}
	r, err := c.App.PlanLoadCost(c.Context, p.ID, nil)
	if err != nil {
		return nil, err
	}
	if args.Bool("json") {
		data, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return nil, err
		}
		return &render.Doc{Raw: string(data) + "\n"}, nil
	}
	rangeText := func(low, high int64) string { return render.Size(low) + "–" + render.Size(high) }
	d, root := render.New("plan:" + p.Name + " load cost (estimate)")
	root.KV("mods", fmt.Sprintf("%d (%d measured, %d cached)", r.Mods, r.Measured, r.Cached))
	root.KV("RAM, baseline + mods", rangeText(r.RAMLow+r.Baseline.RAM, r.RAMHigh+r.Baseline.RAM))
	root.KV("VRAM, baseline + mods", rangeText(r.VRAMLow+r.Baseline.VRAM, r.VRAMHigh+r.Baseline.VRAM))
	root.KV("RAM, loading budget", rangeText(r.LoadingRAMLow+r.Baseline.RAM, r.LoadingRAMHigh+r.Baseline.RAM))
	capacity := r.Capacity
	hardware := capacity.Hardware
	ramAvailable, vramAvailable := "unknown", "unknown"
	if hardware.TotalRAM > 0 {
		ramAvailable = render.Size(capacity.RAMBudget) + " / total " + render.Size(hardware.TotalRAM)
	}
	if hardware.VRAM() > 0 {
		vramAvailable = render.Size(hardware.VRAM())
	}
	root.KV("RAM, available now", ramAvailable+" ["+capacity.RAMSeverity+"]")
	root.KV("VRAM, dedicated capacity", vramAvailable+" ["+capacity.VRAMSeverity+"]")
	if len(hardware.Adapters) > 0 {
		root.KV("GPU candidate", hardware.Adapters[0].Name+" (OS adapter preference; game selection unverified)")
	}
	root.KV("capacity caveat", "Unity 1.6 RAM calibrated in VM; 1.6 VRAM and 1.7 loading reserve uncalibrated. Excludes saves and arbitrary mod allocations; shortage is a risk, not proof of a crash.")
	root.KV("RAM, mods only", rangeText(r.RAMLow, r.RAMHigh))
	root.KV("VRAM, mods only", rangeText(r.VRAMLow, r.VRAMHigh))
	root.KV("vanilla + all DLC baseline", "RAM "+render.Size(r.Baseline.RAM)+", VRAM "+render.Size(r.Baseline.VRAM))
	rows := [][]string{{"category", "files / mods", "disk bytes (DDS payload)", "RAM", "VRAM"}}
	for _, p := range r.Parts {
		rows = append(rows, []string{p.Key, fmt.Sprint(p.Count), render.Size(p.Bytes), rangeText(p.RAMLow, p.RAMHigh), rangeText(p.VRAMLow, p.VRAMHigh)})
	}
	root.Section("parts (mods only)").AddColumns(rows)
	rows = [][]string{{"mod", "ref", "RAM high", "VRAM high"}}
	for _, m := range r.Top {
		rows = append(rows, []string{m.Name, m.Ref, render.Size(m.RAMHigh), render.Size(m.VRAMHigh)})
	}
	root.Section("top mods (five by VRAM, five by RAM; duplicates omitted)").AddColumns(rows)
	if args.Bool("mods") {
		rows = [][]string{{"load order", "mod", "ref", "RAM", "VRAM"}}
		for i, m := range r.PerMod {
			rows = append(rows, []string{fmt.Sprint(i + 1), m.Name, m.Ref, rangeText(m.Est.RAMLow, m.Est.RAMHigh), rangeText(m.Est.VRAMLow, m.Est.VRAMHigh)})
		}
		root.Section("per-mod estimates (mods only, load order)").AddColumns(rows)
	}
	return d, nil
}
