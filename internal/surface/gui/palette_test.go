package gui

import (
	"fmt"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 16 §3.5: the panel's own entries over a thousand Mods, many holding the typed letters in
// order: a typo still finds its command first, and Mods still jump.
func TestPaletteLibrary(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	api.Board()
	api.mu.Lock()
	for i, name := range testkit.ModNames(1000) {
		api.board.Cols[view.Inactive] = append(api.board.Cols[view.Inactive], view.Item{ID: fmt.Sprintf("syn%d", i), Name: name, Package: "syn.mod", Author: "someone"})
	}
	api.mu.Unlock()
	for _, c := range []struct{ q, kind, first string }{
		{"sort", "cmd", "sort"},
		{"sotr", "cmd", "sort"},
		{"subscribe", "cmd", "subscribe"},
		{"subscirbe", "cmd", "subscribe"},
		{"analyze", "cmd", "analyze"},
		{"hug lib", "mod", "HugsLib"},
		{"harmny", "mod", "Harmony"},
	} {
		got := api.Palette(c.q)
		if len(got) == 0 {
			t.Errorf("%q: nothing", c.q)
			continue
		}
		top := got[0]
		if c.kind == "cmd" && (top.Kind != view.PalCommand || top.ID != c.first) || c.kind == "mod" && (top.Kind != view.PalMod || top.Name != c.first) {
			t.Errorf("%q: first %q (%s), want %s", c.q, top.Name, top.ID, c.first)
		}
	}
}
