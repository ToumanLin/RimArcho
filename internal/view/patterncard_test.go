package view

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/patterns"
)

// Every summary and cause the card can show, and every kind of fix, has its Chinese text.
func TestPatternTextCovered(t *testing.T) {
	t.Parallel()
	for _, p := range patterns.Load().Rules {
		if p.Fallback {
			continue
		}
		for _, s := range append([]string{p.Summary}, p.Causes...) {
			if _, ok := zhText[s]; !ok {
				t.Errorf("%s: no Chinese text for %q", p.ID, s)
			}
		}
		for _, f := range p.Fixes {
			w, ok := fixWords[f]
			if !ok {
				t.Errorf("%s: no words for fix %q", p.ID, f)
			} else if _, ok := zhText[w]; !ok {
				t.Errorf("no Chinese text for %q", w)
			}
		}
	}
}
