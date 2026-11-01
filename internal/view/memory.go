package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// MemoryNeed keeps the comparison and the severity common to GUI and TUI.
func MemoryNeed(c app.MemoryAssessment, plan int64) Need {
	it := Need{Group: GroupList, Section: SectionHint, Status: StatusHint, Kind: "memory.capacity", code: "memory.capacity", Title: T("RAM / VRAM"),
		Actions: []Action{{Kind: ActView, Key: "v", Label: T("refresh memory estimate"), Argv: []string{"plan", "cost", fmt.Sprint(plan)}}}}
	ram, vram := T("not measured"), T("not measured")
	if c.Estimated {
		ram = costRange(c.RAMLow, c.RAMHigh)
		vram = costRange(c.VRAMLow, c.VRAMHigh)
	}
	available, total := T("unavailable"), T("unavailable")
	if c.Hardware.TotalRAM > 0 {
		available = SizeText(c.RAMBudget)
		total = SizeText(c.Hardware.TotalRAM)
	}
	video := T("unavailable")
	if c.Hardware.VRAM() > 0 {
		video = SizeText(c.Hardware.VRAM())
	}
	lines := []string{
		Tf("RAM: loading needs %s; available now %s of %s", ram, available, total),
		Tf("VRAM: estimated %s; dedicated capacity %s", vram, video),
	}
	if c.Estimated && c.VRAMInRAMHigh > 0 {
		lines = append(lines, Tf("RAM includes %s of VRAM held in system memory on this computer.", costRange(c.VRAMInRAMLow, c.VRAMInRAMHigh)))
	}
	if c.RAMSeverity == "severe" {
		it.Status = StatusErr
		it.Section = SectionBreak
		it.Counted = true
		lines = append(lines, T("RAM may be insufficient: close other apps or reduce the list before launching."))
	} else if c.RAMSeverity == "warning" || c.VRAMSeverity == "warning" {
		it.Status = StatusWarn
		it.Section = SectionBreak
		it.Counted = true
	}
	if c.RAMSeverity == "warning" {
		lines = append(lines, T("RAM is tight: closing other apps leaves more room."))
	}
	if c.VRAMSeverity == "warning" {
		lines = append(lines, T("VRAM may be insufficient: optimize textures or reduce texture mods."))
	}
	if len(c.Hardware.Adapters) > 0 {
		lines = append(lines, Tf("GPU: %s (by the system's preference)", c.Hardware.Adapters[0].Name))
	}
	if it.Counted {
		lines = append(lines, T("Saves and memory mods allocate themselves are not counted; a shortfall is a risk, not a certain crash."))
	}
	it.Evidence = evidence(EvInferred, T("file estimates and OS memory snapshot"))
	it.Why = strings.Join(lines, "\n")
	it.Text = it.Title + ": " + strings.Join(lines, "; ")
	identifyNeed(&it)
	return it
}

func RefreshMemory(a *app.App, st *app.LoadoutState, old *Needs) *Needs {
	if st == nil {
		return old
	}
	n := *old
	n.Items = nil
	for _, it := range old.Items {
		if it.Kind != "memory.capacity" {
			n.Items = append(n.Items, it)
		}
	}
	c := a.CachedMemoryAssessment(st)
	n.Memory = &c
	n.Items = append(n.Items, MemoryNeed(c, st.Plan.ID))
	sort.SliceStable(n.Items, func(i, j int) bool {
		if n.Items[i].Section != n.Items[j].Section {
			return n.Items[i].Section < n.Items[j].Section
		}
		return n.Items[i].Status != StatusHint && n.Items[j].Status == StatusHint
	})
	indexNeeds(&n)
	return &n
}

// MemoryLaunch binds a cached comparison to the exact environment/list snapshot.
// It performs no file measurement on the page's paint or launch paths.
type MemoryLaunch struct {
	Env        string               `json:"env"`
	Plan       int64                `json:"plan"`
	Token      string               `json:"token"`
	Assessment app.MemoryAssessment `json:"assessment"`
	Needed     string               `json:"needed"`
	Available  string               `json:"available"`
}

func BuildMemoryLaunch(a *app.App, st *app.LoadoutState) *MemoryLaunch {
	if st == nil || st.Plan == nil {
		return nil
	}
	c := a.CachedMemoryAssessment(st)
	return &MemoryLaunch{Env: st.Ix.Env.Name, Plan: st.Plan.ID, Token: app.MemoryFingerprint(st), Assessment: c, Needed: costRange(c.RAMLow, c.RAMHigh), Available: SizeText(c.RAMBudget)}
}
