package cli

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// 06 §7.2: the interface uses the player's words. This package links every surface, so it
// sees every translation table, the same as the program.
func TestWording(t *testing.T) {
	for _, p := range view.WordingProblems() {
		t.Error(p)
	}
}
