package tui

import (
	"strings"

	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// A Mod library row with several copies keeps its width and its copies count.
func TestFitDimKeepsWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	for _, w := range []int{40, 28, 12} {
		got := fitDim("▸ [1.6] JobsLimitsTweak  4 copies", "4 copies", w)
		if lipgloss.Width(got) != w {
			t.Errorf("width %d: %d cells: %q", w, lipgloss.Width(got), got)
		}
	}
	if got := fitDim("▸ [1.6] JobsLimitsTweak  4 copies", "4 copies", 40); !strings.Contains(ansi.Strip(got), "4 copies") {
		t.Errorf("copies count lost: %q", got)
	}
}
