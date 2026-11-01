package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadcost"
)

type LoadCostRow struct {
	Category string `json:"category"`
	Files    string `json:"files"`
	Disk     string `json:"disk"`
	RAM      string `json:"ram"`
	VRAM     string `json:"vram"`
}

type LoadCostTop struct {
	Name string `json:"name"`
	Ref  string `json:"ref"`
	Page string `json:"page"`
	RAM  string `json:"ram"`
	VRAM string `json:"vram"`
}

// PlanCost contains only display text and navigation targets.
type PlanCost struct {
	Summary string        `json:"summary"`
	Loading string        `json:"loadingBudget"`
	Detail  string        `json:"detail"`
	Mods    string        `json:"mods"`
	Parts   []LoadCostRow `json:"parts"`
	Top     []LoadCostTop `json:"top"`
	Notes   []string      `json:"notes"`
}

func costRange(low, high int64) string {
	if low == 0 && high == 0 {
		return "—"
	}
	return SizeText(low) + "–" + SizeText(high)
}

// PlanLoadCost renders a completed measurement without reading the filesystem.
func PlanLoadCost(r *app.LoadCost) *PlanCost {
	p := &PlanCost{
		Summary: Tf("RAM about %s  VRAM about %s", costRange(r.RAMLow+r.Baseline.RAM, r.RAMHigh+r.Baseline.RAM), costRange(r.VRAMLow+r.Baseline.VRAM, r.VRAMHigh+r.Baseline.VRAM)),
		Loading: Tf("Loading RAM budget %s", costRange(r.LoadingRAMLow+r.Baseline.RAM, r.LoadingRAMHigh+r.Baseline.RAM)),
		Detail:  Tf("Vanilla and DLC about %s / %s; mods about %s / %s", SizeText(r.Baseline.RAM), SizeText(r.Baseline.VRAM), costRange(r.RAMLow, r.RAMHigh), costRange(r.VRAMLow, r.VRAMHigh)),
		Mods:    Tf("%d mods", r.Mods),
		Parts:   []LoadCostRow{}, Top: []LoadCostTop{},
	}
	categories := map[string]string{"textures": "textures", "dds": "DDS", "sounds": "sounds", "defs": "Defs", "patches": "Patches", "assemblies": "assemblies", "mod overhead": "mod overhead"}
	for _, part := range r.Parts {
		count := Tf("%d files", part.Count)
		if part.Key == "mod overhead" {
			count = Tf("%d mods", part.Count)
		}
		p.Parts = append(p.Parts, LoadCostRow{T(categories[part.Key]), count, SizeText(part.Bytes), costRange(part.RAMLow, part.RAMHigh), costRange(part.VRAMLow, part.VRAMHigh)})
	}
	for _, m := range r.Top {
		p.Top = append(p.Top, LoadCostTop{m.Name, m.Ref, m.Page, SizeText(m.RAMHigh), SizeText(m.VRAMHigh)})
	}
	if r.Game == loadcost.Game17 {
		p.Notes = []string{
			T("Textures: the range is measured on 1.7 with 100–484 mods (0.9–2.6× the BC7 + mipmap size in VRAM, 1.5–3.5× in RAM); first launches sit lower, later launches higher."),
			T("The game uploads textures on demand and starts evicting at a share of the graphics card's memory, so a smaller card shows less VRAM."),
			T("Sounds: decoded clips up to 2 MiB stay in RAM; larger clips stream."),
			T("The 1.7 loading reserve is uncalibrated."),
		}
	} else {
		p.Notes = []string{
			T("Textures use DXT or RGBA32 with mipmaps; all textures load into VRAM. The upper bound includes static atlas copies."),
			T("1.6 RAM is calibrated with up to 7.5 GiB of textures; larger loads are extrapolated. VRAM follows loading rules only."),
			T("Sounds: files up to 300 KiB are decoded into RAM; larger clips stream."),
		}
	}
	p.Notes = append(p.Notes,
		T("Def and Patch XML estimates cover objects retained after loading; peak RAM during loading is higher."),
		T("Assemblies stay in RAM after loading."),
		T("RAM is the game's committed private memory; Task Manager's Memory column shows only the part in physical memory, usually less."),
		T("Excludes saves, maps and growth during play."),
		T("JPG/PSD dimensions and audio without readable WAV headers are approximated; DDS disk size excludes its header."),
	)
	return p
}
