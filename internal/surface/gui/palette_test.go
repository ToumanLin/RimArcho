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
		got := api.Palette(c.q, "", nil)
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

// 16 §3.5: opened on a page, the empty panel suggests that page's own commands, then its
// parts (the page's areas, as the window names them), before everything else.
func TestPalettePage(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	api.Board()
	for _, page := range []string{view.EditorPage, "home", "needs", view.LibraryPage, "settings", "plan-import"} {
		got := api.Palette("", page, []string{"first area", " "})
		own := view.PageCommands(page)
		if len(own) == 0 || len(got) <= len(own) {
			t.Fatalf("%s: %d page commands, %d results", page, len(own), len(got))
		}
		for i, id := range own {
			if got[i].Kind != view.PalCommand || got[i].ID != id || !got[i].Own {
				t.Errorf("%s: #%d %q, want %q", page, i, got[i].ID, id)
			}
		}
		if p := got[len(own)]; p.Kind != view.PalPart || p.ID != "0" || p.Name != "first area" {
			t.Errorf("%s: after the commands %+v, want the part", page, p)
		}
		for _, e := range got {
			if e.Kind == view.PalPart && e.ID == "1" {
				t.Errorf("%s: a blank part is listed", page)
			}
		}
	}
}
