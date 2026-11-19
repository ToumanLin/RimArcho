package view_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Disabled's orders (16 §2.5): A-Z, last updated (Workshop), files modified, added; the time
// orders are newest first, Mods without the time follow by name.
func TestDisabledSort(t *testing.T) {
	e := testkit.New(t)
	a := e.AddWorkshop("6100", testkit.Sample("tester.alpha", "Alpha"))
	b := e.AddWorkshop("6200", testkit.Sample("tester.beta", "Beta"))
	e.SetTimeUpdated("6100", 1_700_000_000)
	e.SetTimeUpdated("6200", 1_800_000_000)
	g := filepath.Join(e.Game, "Mods", "Gamma")
	e.CopyTree(a, g)
	e.WriteFile(filepath.Join(g, "About", "About.xml"), testkit.Sample("tester.gamma", "Gamma").AboutXML())
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("scan")
	old, recent := time.Unix(1_600_000_000, 0), time.Unix(1_900_000_000, 0)
	for dir, at := range map[string]time.Time{a: old, b: old, g: recent} {
		if err := os.Chtimes(dir, at, at); err != nil {
			t.Fatal(err)
		}
	}
	order := func(by string) []string {
		t.Helper()
		if err := e.App.Store.SetMeta(view.DisabledSortKey, by); err != nil {
			t.Fatal(err)
		}
		bd, err := view.BuildBoard(e.App)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, it := range bd.Cols[view.Inactive] {
			if slices.Contains([]string{"Alpha", "Beta", "Gamma"}, it.Name) {
				out = append(out, it.Name)
			}
		}
		return out
	}
	for by, want := range map[string][]string{
		"":         {"Alpha", "Beta", "Gamma"},
		"name":     {"Alpha", "Beta", "Gamma"},
		"updated":  {"Beta", "Alpha", "Gamma"},
		"modified": {"Gamma", "Alpha", "Beta"},
	} {
		if got := order(by); !slices.Equal(got, want) {
			t.Errorf("sort %q = %v, want %v", by, got, want)
		}
	}
}

// version:current is the game's own version (16 §3.7).
func TestVersionCurrent(t *testing.T) {
	e := libEnv(t)
	bd, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	cur := view.FilterBoard(bd, "version:current", nil, view.MatchFacts{}, false, "1.6")
	same := view.FilterBoard(bd, "version:1.6", nil, view.MatchFacts{}, false, "1.6")
	if len(same.Rows[view.Inactive]) == 0 || !slices.Equal(cur.Rows[view.Inactive], same.Rows[view.Inactive]) {
		t.Fatalf("version:current = %v, version:1.6 = %v", cur.Rows, same.Rows)
	}
	neg := view.FilterBoard(bd, "", []string{"-version:current"}, view.MatchFacts{}, false, "1.6")
	if len(neg.Rows[view.Inactive])+len(same.Rows[view.Inactive]) != len(bd.Cols[view.Inactive]) {
		t.Fatalf("-version:current = %v", neg.Rows)
	}
}
