package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestIconSetting(t *testing.T) {
	old := ic
	t.Cleanup(func() { ic = old })
	t.Setenv("TERM_PROGRAM", "test-terminal")
	for _, setting := range []string{"", "auto", "compat", "invalid", "nerd"} {
		SetIcons(setting)
		want := compatIcons
		if setting == "nerd" {
			want = nerdIcons
		}
		if ic != want {
			t.Fatalf("%q selected the wrong icons", setting)
		}
	}
}

func TestColorCapabilities(t *testing.T) {
	old := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	t.Setenv("TERM_PROGRAM", "test-terminal")
	for _, tc := range []struct {
		color, session string
		want           termenv.Profile
	}{
		{"", "", termenv.Ascii},
		{"truecolor", "", termenv.TrueColor},
		{"24bit", "", termenv.TrueColor},
		{"", "test-session", termenv.TrueColor},
	} {
		t.Setenv("COLORTERM", tc.color)
		t.Setenv("WT_SESSION", tc.session)
		lipgloss.SetColorProfile(termenv.Ascii)
		SetColors()
		if got := lipgloss.ColorProfile(); got != tc.want {
			t.Fatalf("COLORTERM=%q WT_SESSION=%q: %v, want %v", tc.color, tc.session, got, tc.want)
		}
	}
}
