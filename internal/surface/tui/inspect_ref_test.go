package tui

import (
	"slices"
	"strings"
	"testing"
)

// enter follows the first reference in a line; a printed command writes instance references
// without "#" (05 §3.1 rule 11), and those are followed when the index knows them.
func TestLineRefBareCommandReference(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]string{}
	lin := ""
	for _, m := range ix.Mods {
		refs[m.Name] = ix.Ref(m.ID)
		if m.Name == "Alpha" {
			lin = ix.LRef(m.LineageID)
		}
	}
	alpha, harmony := refs["Alpha"], refs["Harmony"]
	bare := strings.TrimPrefix(alpha, "#")
	for _, c := range []struct {
		line string
		want []string
	}{
		{"      fix: archo enable " + bare, []string{"mod", bare}},
		{"  changes: 1, latest discovered 2026-10-01 (archo changes " + bare + ")", []string{"mod", bare}},
		{"  archo why " + bare + " " + strings.TrimPrefix(harmony, "#"), []string{"mod", bare}},
		// the first reference wins, in prose or in a command
		{"error: " + harmony + " is not enabled (archo enable " + bare + ")", []string{"mod", harmony}},
		{"  archo lineage merge " + lin + " " + bare, []string{"list", "--filter", "lineage:" + lin[2:]}},
		{"  archo problems --severity error", nil}, // command words are not mods
	} {
		if got := lineRef(c.line, ix); !slices.Equal(got, c.want) {
			t.Errorf("lineRef(%q) = %q, want %q", c.line, got, c.want)
		}
	}
	if got := lineRef("      fix: archo enable "+bare, nil); got != nil {
		t.Errorf("without an index lineRef = %q", got)
	}
}
