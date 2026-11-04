package gui

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 06 §4.2: the full detail's copies part is the mod's Lineage row as Mod library builds it,
// found by the instance id or a copy ref, with the op table its codes refer to.
func modCopies(t *testing.T, _ *testkit.Env, api *API) {
	var alpha string
	for _, c := range api.Board().Cols {
		for _, it := range c {
			if it.Name == "Alpha" {
				alpha = it.ID
			}
		}
	}
	if alpha == "" {
		t.Fatal("no Alpha on the board")
	}
	mc, err := api.ModCopies(alpha)
	if err != nil {
		t.Fatal(err)
	}
	if mc.Row == nil || mc.Row.Name != "Alpha" || len(mc.Row.Insts) == 0 || len(mc.Ops) == 0 {
		t.Fatalf("copies %+v", mc)
	}
	c := mc.Row.Insts[0]
	if !c.InPlan || len(c.Ops) == 0 || c.Size == "" {
		t.Fatalf("copy %+v", c)
	}
	// by the copy's ref too, and nothing for an unknown id
	if by, _ := api.ModCopies(c.Ref); by.Row == nil || by.Row.Name != "Alpha" {
		t.Fatalf("by ref: %+v", by)
	}
	if none, _ := api.ModCopies("nothing"); none.Row != nil {
		t.Fatalf("unknown id found %+v", none.Row)
	}
}
