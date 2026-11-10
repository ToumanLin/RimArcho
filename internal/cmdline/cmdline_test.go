package cmdline

import (
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Completing after "--" counts positional words the way the parser does and never
// indexes past them (a "config set -- " line once panicked).
func TestCompleteAfterDoubleDash(t *testing.T) {
	e := testkit.New(t)
	labels := func(line string) []string {
		cs, _, _ := Complete(Context{App: e.App}, line, len([]rune(line)))
		var out []string
		for _, c := range cs {
			out = append(out, c.Label)
		}
		return out
	}
	for _, line := range []string{"config set -- ", "config set --env x ", "config set -- -- "} {
		labels(line) // must not panic
	}
	if got := labels("config set -- ui.theme "); !slices.Contains(got, "dark") {
		t.Errorf("values after --: %v, want ui.theme choices", got)
	}
	if got := labels("config set --env x ui.theme "); !slices.Contains(got, "dark") {
		t.Errorf("values after --env x: %v, want ui.theme choices", got)
	}
}
