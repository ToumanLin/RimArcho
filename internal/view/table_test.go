package view

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// Savings refer to the PNGs the pending conversion replaces (06 §2.13, 06 §3.9).
func TestTextureTableSortValues(t *testing.T) {
	m := &app.TextureMod{Mod: &store.Mod{Name: "Alpha"}, Ref: "alpha", PNG: 10, PNGBytes: 9000,
		Plan: &textures.Plan{Est: textures.Estimate{Files: 4, PNGBytes: 5000, DDSBytes: 2000}}}
	r := TextureRowOf(m)
	if r.PNGCount != 10 || r.PNGBytes != 9000 || r.EstimateBytes != 2000 || r.SavedBytes != 3000 ||
		r.CurrentSize != SizeText(9000) || r.Estimate != SizeText(2000) {
		t.Fatalf("texture sort values: %+v", r)
	}
	m.Plan.Est.DDSBytes = 7000
	if r := TextureRowOf(m); r.SavedBytes != -2000 {
		t.Fatalf("a growing conversion must sort below savings: %+v", r)
	}
}
