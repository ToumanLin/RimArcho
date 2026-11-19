package view

import (
	"fmt"
	"runtime"
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
	if runtime.GOOS != "windows" {
		if c.Hardware.TotalRAM <= 0 {
			lines[0] = Tf("RAM: loading needs %s; available memory and total capacity are unavailable", ram)
		} else if c.Hardware.RAMAvailableUnknown {
			lines[0] = Tf("RAM: loading needs %s; available memory is unavailable; total capacity %s", ram, total)
		}
		if c.Hardware.VRAM() <= 0 {
			lines[1] = Tf("VRAM: estimated %s; dedicated capacity is unknown", vram)
		}
	}
	if c.Estimated && c.VRAMInRAMHigh > 0 {
		lines = append(lines, Tf("RAM includes %s of VRAM held in system memory on this computer.", costRange(c.VRAMInRAMLow, c.VRAMInRAMHigh)))
	}
	if c.Estimated && c.TexturesOnDemand > 0 {
		lines = append(lines, Tf("The game keeps only what fits this card; about %s of textures load from the disk cache when drawn, which may stutter but does not stop the game.", SizeText(c.TexturesOnDemand)))
	}
	// Only a shortage of committable memory breaks the game; past available physical
	// RAM it pages, which is slow but runs, so a warning stays in worth knowing.
	if c.RAMSeverity == "severe" {
		it.Status = StatusErr
		it.Section = SectionBreak
		it.Counted = true
		lines = append(lines, T("RAM may be insufficient: close other apps or reduce the list before launching."))
	} else if c.RAMSeverity == "warning" || c.VRAMSeverity == "warning" {
		it.Status = StatusWarn
	}
	if c.RAMSeverity == "warning" {
		lines = append(lines, T("RAM is tight: closing other apps leaves more room."))
	}
	if c.VRAMSeverity == "warning" {
		lines = append(lines, T("VRAM may be insufficient: optimize textures or reduce texture mods."))
	}
	// the GUI draws lines[0], lines[1] and the GPU as bars and shows the rest (16 §7 principle 9)
	shown := append([]string(nil), lines[2:]...)
	gpu := ""
	if len(c.Hardware.Adapters) > 0 {
		gpu = Tf("GPU: %s (by the system's preference)", c.Hardware.Adapters[0].Name)
		lines = append(lines, gpu)
	}
	if it.Status != StatusHint {
		note := T("Saves and memory mods allocate themselves are not counted; a shortfall is a risk, not a certain crash.")
		lines = append(lines, note)
		shown = append(shown, note)
	}
	it.Evidence = evidence(EvInferred, T("file estimates and OS memory snapshot"))
	it.Why = strings.Join(lines, "\n")
	it.Bars = []MemoryBar{ramBar(c, lines[0]), vramBar(c, joinSentences(nonEmpty(lines[1], gpu)))}
	it.BarsWhy = strings.Join(shown, "\n")
	it.Text = Tf("%s: %s", it.Title, joinSentences(lines))
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

// MemoryBar is one resource drawn as a stacked bar (16 §7 principle 9): what is in use
// now, the estimate's parts after it, and the free rest of Total. The bytes are for the
// drawing only; Text is the sentence it stands for, the bar's accessible name and tooltip,
// so nothing the sentence says is lost.
type MemoryBar struct {
	Label string    `json:"label"`          // RAM, VRAM
	Total int64     `json:"total"`          // capacity; 0 when unknown
	Used  int64     `json:"used,omitempty"` // in use by everything else now (RAM only)
	Parts []BarPart `json:"parts,omitempty"`
	Size  string    `json:"size"`           // printed after the bar: the capacity, or the estimate when it is unknown
	Note  string    `json:"note,omitempty"` // small text after it: the graphics card
	Text  string    `json:"text"`
}

// BarPart is one segment of the estimate: Low solid, then up to High hatched. Kind is
// "need" (one estimate), "game" (the game and DLC) or "mods".
type BarPart struct {
	Kind string `json:"kind"`
	Low  int64  `json:"low"`
	High int64  `json:"high"`
}

// ramBar: in use is what is not available now; unknown (0) when the OS gives no figure.
// No parts: the assessment's estimate as one "need" part.
func ramBar(c app.MemoryAssessment, text string, parts ...BarPart) MemoryBar {
	b := MemoryBar{Label: T("RAM"), Total: c.Hardware.TotalRAM, Text: text, Parts: parts}
	if b.Total > 0 {
		b.Size = SizeText(b.Total)
		if !c.Hardware.RAMAvailableUnknown && c.Hardware.AvailableRAM > 0 && c.Hardware.AvailableRAM < b.Total {
			b.Used = b.Total - c.Hardware.AvailableRAM
		}
	}
	if parts == nil && c.Estimated {
		b.Parts = []BarPart{{Kind: "need", Low: c.RAMLow, High: c.RAMHigh}}
	}
	return b.sized()
}

func vramBar(c app.MemoryAssessment, text string, parts ...BarPart) MemoryBar {
	b := MemoryBar{Label: T("VRAM"), Total: c.Hardware.VRAM(), Text: text, Parts: parts}
	if b.Total > 0 {
		b.Size = SizeText(b.Total)
	}
	if len(c.Hardware.Adapters) > 0 {
		b.Note = c.Hardware.Adapters[0].Name
	}
	if parts == nil && c.Estimated {
		b.Parts = []BarPart{{Kind: "need", Low: c.VRAMLow, High: c.VRAMHigh}}
	}
	return b.sized()
}

// sized: with the capacity unknown nothing can be drawn to scale, so the bar prints the
// estimate instead (the GUI draws an empty, dashed track).
func (b MemoryBar) sized() MemoryBar {
	if b.Total <= 0 && len(b.Parts) > 0 {
		var low, high int64
		for _, p := range b.Parts {
			low += p.Low
			high += p.High
		}
		b.Size = costRange(low, high)
	}
	return b
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
	// RAM: the launch warning's bar, named by Can it run's RAM sentence
	RAM MemoryBar `json:"ram"`
}

func BuildMemoryLaunch(a *app.App, st *app.LoadoutState) *MemoryLaunch {
	if st == nil || st.Plan == nil {
		return nil
	}
	c := a.CachedMemoryAssessment(st)
	return &MemoryLaunch{Env: st.Ix.Env.Name, Plan: st.Plan.ID, Token: app.MemoryFingerprint(st), Assessment: c, Needed: costRange(c.RAMLow, c.RAMHigh), Available: SizeText(c.RAMBudget),
		RAM: MemoryNeed(c, st.Plan.ID).Bars[0]}
}

// joinSentences puts the lines on one line, for the TUI's item text: the language's "; "
// between them, or a space after a line that already ends a sentence (VM 2026-11-10: the
// Chinese item put "; " right after a full stop).
func joinSentences(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			prev := lines[i-1]
			switch {
			case strings.HasSuffix(prev, "\u3002"), strings.HasSuffix(prev, "\uff1b"): // full-width full stop, semicolon
			case strings.HasSuffix(prev, "."):
				b.WriteString(" ")
			default:
				b.WriteString(T("; "))
			}
		}
		b.WriteString(l)
	}
	return b.String()
}
