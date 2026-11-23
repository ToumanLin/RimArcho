package modchanges_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/modchanges"
)

// The Launch page's changes line counts in English singular and plural (it read "1 mods
// changed" in the 1.1 VM round).
func TestHomeTextSingular(t *testing.T) {
	for _, c := range []struct {
		count, launch int
		want          string
	}{
		{0, 0, "nothing changed"},
		{1, 0, "1 mod changed"},
		{1, 1, "1 mod changed, it needs attention"},
		{3, 0, "3 mods changed"},
		{3, 1, "3 mods changed, 1 need attention"},
	} {
		if got := modchanges.HomeText(c.count, c.launch); got != c.want {
			t.Errorf("%d, %d: %q, want %q", c.count, c.launch, got, c.want)
		}
	}
}
