package view

import (
	"context"
	"sort"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// Texture optimisation (08 §5.2, 06 §2.13): the page that converts the current plan's (or every mod's)
// PNG textures to DDS next to them, and takes them back; Mod library does the same per copy
// (OpTextures, OpTexClean), packs on their own copies (PackPreview's textures step).

// TextureScales are the choices offered, full size first.
var TextureScales = []int{100, 80, 75, 60, 50, 40, 25, 20}

// TextureScaleLabel names a choice: full size or a percent.
func TextureScaleLabel(s int) string {
	if s <= 0 || s >= 100 {
		return T("full size")
	}
	return Tf("%d%%", s)
}

// TexturePage is the Texture optimisation page.
type TexturePage struct {
	Loading bool `json:"loading,omitempty"`
	// Measured: the textures were read (Scope, Scale say with what). The page opens with
	// only the encoder and the mods; the player starts the read (06 §2.13).
	Measured bool     `json:"measured,omitempty"`
	Scope    string   `json:"scope"` // app.TexPlan or app.TexAll
	Scale    int      `json:"scale"`
	Scales   []int    `json:"scales"`
	Labels   []string `json:"labels"` // of Scales
	// The encoder: ready, or what is wrong and the offer to download it.
	Tool     string `json:"tool"`
	ToolPath string `json:"toolPath,omitempty"`
	ToolOK   bool   `json:"toolOk"`
	Offer    string `json:"offer,omitempty"`   // "todds 0.4.1  3.7 MB  from github.com/…"
	OfferTo  string `json:"offerTo,omitempty"` // where it goes
	// What is there now.
	Mods     int    `json:"mods"`
	PNG      string `json:"png"`      // "13208 PNG  412 MB"
	Ours     string `json:"ours"`     // "3201 DDS  1.2 GB" ("" none)
	OursN    int    `json:"oursN"`    // files
	Stale    int    `json:"stale"`    // DDS whose PNG changed or is gone
	StaleMod int    `json:"staleMod"` // mods with any
	// StaleText says so (Go builds it: its translation reorders the numbers)
	StaleText string `json:"staleText,omitempty"`
	// What a run with Scale would do.
	Convert   int          `json:"convert"`
	EstFacts  []string     `json:"estFacts"` // the DDS size, video memory, what stays
	UpToDate  int          `json:"upToDate"`
	LeftAlone int          `json:"leftAlone"`
	Rows      []TextureRow `json:"rows"`
}

// TextureRow is one mod of the page: the biggest first.
type TextureRow struct {
	PNGCount      int    `json:"pngCount"`
	PNGBytes      int64  `json:"pngBytes"`
	CurrentSize   string `json:"currentSize"`
	EstimateBytes int64  `json:"estimateBytes"`
	SavedBytes    int64  `json:"savedBytes"`
	Pending       bool   `json:"pending,omitempty"`
	Estimate      string `json:"estimate"`
	Ref           string `json:"ref"`
	Name          string `json:"name"`
	PNG           string `json:"png"`
	Convert       int    `json:"convert"`
	DDS           string `json:"dds,omitempty"` // RimArcho's DDS there now
	Stale         int    `json:"stale,omitempty"`
	Own           int    `json:"own,omitempty"` // DDS of the mod's own
}

// TexturePageLoading offers choices before any disk measurements finish (06 §3.1).
func TexturePageLoading(scope string, scale int) *TexturePage {
	p := &TexturePage{Scope: scope, Scale: textures.Options{Scale: scale}.Normal().Scale, Scales: TextureScales, Loading: true}
	for _, s := range TextureScales {
		p.Labels = append(p.Labels, TextureScaleLabel(s))
	}
	return p
}

// TextureRowOf formats one completed measurement (06 §2.13).
func TextureRowOf(m *app.TextureMod) TextureRow {
	row := TextureRow{Ref: m.Ref, Name: m.Mod.DisplayName(), PNG: Tf("%d  %s", m.PNG, SizeText(m.PNGBytes)), PNGCount: m.PNG, PNGBytes: m.PNGBytes, CurrentSize: SizeText(m.PNGBytes), Stale: m.Stale + m.Orphans, Own: m.Foreign}
	if m.Plan != nil {
		row.Convert = m.Plan.Est.Files
		row.Estimate = SizeText(m.Plan.Est.DDSBytes)
		row.EstimateBytes = m.Plan.Est.DDSBytes
		row.SavedBytes = m.Plan.Est.PNGBytes - m.Plan.Est.DDSBytes
	}
	if m.Ours > 0 {
		row.DDS = Tf("%d  %s", m.Ours, SizeText(m.OursSize))
	}
	return row
}

// TexturePageOf builds the page for scope at scale.
func TexturePageOf(a *app.App, scope string, scale int) (*TexturePage, error) {
	return TexturePageContext(context.Background(), a, scope, scale)
}

func TexturePageContext(ctx context.Context, a *app.App, scope string, scale int) (*TexturePage, error) {
	if scope != app.TexAll {
		scope = app.TexPlan
	}
	o := textures.Options{Scale: scale}.Normal()
	r, err := a.TextureSummary(ctx, nil, scope, o)
	if err != nil {
		return nil, err
	}
	return TexturePageReport(a, r, scope), nil
}

// TexturePageInfo is the page before any texture is read: the encoder and how many mods
// the scope holds. Reading the textures is the player's call (06 §2.13).
func TexturePageInfo(a *app.App, scope string, scale int, tool *app.TextureTool, mods int) *TexturePage {
	p := TexturePageLoading(scope, scale)
	p.Loading, p.Mods = false, mods
	if tool != nil {
		p.ToolPath = tool.Path
	}
	if t := tool; t != nil && t.Err == "" {
		p.Tool, p.ToolOK = Tf("todds %s  %s", t.Version, t.Path), true
	} else {
		p.Tool = T("the DDS encoder (todds) is not installed")
		if t != nil && t.Path != "" {
			p.Tool = t.Err
		}
		off := a.OfferTodds()
		p.Offer = Tf("todds %s  %s  from %s (MPL-2.0)", off.Version, SizeText(off.Bytes), off.URL)
		p.OfferTo = off.Dir
	}
	return p
}

// TexturePageReport formats an already measured summary, without loading another index.
func TexturePageReport(a *app.App, r *app.TextureReport, scope string) *TexturePage {
	o := r.Options
	p := TexturePageInfo(a, scope, o.Scale, r.Tool, len(r.Mods))
	p.Measured = true
	png, pngBytes, ours, oursBytes, stale := r.Totals()
	p.PNG = Tf("%d PNG  %s", png, SizeText(pngBytes))
	if ours > 0 {
		p.Ours, p.OursN = Tf("%d DDS  %s", ours, SizeText(oursBytes)), ours
	}
	p.Stale = stale
	for _, m := range r.Mods {
		if m.Stale+m.Orphans > 0 {
			p.StaleMod++
		}
		if m.Plan != nil {
			p.UpToDate += m.Plan.Skipped[textures.SkipFresh]
			p.LeftAlone += m.Plan.Skipped[textures.SkipOwnDDS] + m.Plan.Skipped[textures.SkipOtherFmt]
		}
		if m.PNG == 0 && m.Ours == 0 {
			continue
		}
		p.Rows = append(p.Rows, TextureRowOf(m))
	}
	size := map[string]int64{}
	for _, m := range r.Mods {
		size[m.Ref] = m.PNGBytes
	}
	sort.SliceStable(p.Rows, func(i, j int) bool { return size[p.Rows[i].Ref] > size[p.Rows[j].Ref] })
	if p.Stale > 0 {
		p.StaleText = Tf("%d DDS whose PNG changed or is gone, in %d mods: the game may show the old texture", p.Stale, p.StaleMod)
	}
	p.Convert = r.Est.Files
	p.EstFacts = EstimateFacts(r.Est)
	return p
}

// EstimateFacts says what converting costs and saves, in the interface's words.
func EstimateFacts(e textures.Estimate) []string {
	if e.Files == 0 {
		return []string{T("nothing to convert")}
	}
	return []string{
		Tf("%d PNG (%s) → about %s of DDS", e.Files, SizeText(e.PNGBytes), SizeText(e.DDSBytes)),
		Tf("video memory about %s → %s", SizeText(e.VRAMBefore), SizeText(e.VRAMAfter)),
	}
}

// TextureRunText is what a run did, for the status line.
func TextureRunText(r *app.TextureReport) string {
	s := Tf("converted %d textures to DDS (%s written)", r.Converted, SizeText(r.Written))
	if r.Stopped {
		s = Tf("stopped: %d textures converted, %d not", r.Converted, r.Incomplete)
	}
	if len(r.Failed) > 0 {
		s += Tf(", %d failed (they keep their PNG)", len(r.Failed))
	}
	if r.Removed > 0 {
		s += Tf(", %d stale DDS removed", r.Removed)
	}
	return s
}

// TextureProgressText is a run as it goes.
func TextureProgressText(p app.TextureProgress) string {
	s := Tf("%d of %d textures  %d of %d mods", p.Files, p.FilesTotal, p.Mods, p.ModsTotal)
	if p.Mod != "" {
		s += "  " + p.Mod
	}
	return s
}

// TextureFields are a copy's textures line in Mod library's detail ("" none).
func TextureField(i *app.LibInstance) string {
	t := i.Textures
	if t == nil || t.Files == 0 {
		return ""
	}
	s := Tf("%d DDS by RimArcho  %s", t.Files, SizeText(t.Bytes))
	if t.Scale > 0 && t.Scale < 100 {
		s += "  " + Tf("%d%%", t.Scale)
	}
	if t.Stale {
		s += "  " + T("the mod changed since: convert again")
	}
	return s
}
