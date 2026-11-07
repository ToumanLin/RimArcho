package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/muesli/termenv"
)

// Keep ANSI in the snapshots: stripping it would hide selection regressions.
func TestSelectionRows(t *testing.T) {
	oldProfile, oldIcons := lipgloss.ColorProfile(), ic
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile); ic = oldIcons })
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	var snapshot strings.Builder
	states := []struct {
		name                      string
		selected, cursor, focused bool
		bg                        string
	}{
		{"plain", false, false, true, ""},
		{"cursor", false, true, true, "48;2;26;28;36"},
		{"selected", true, false, true, "48;2;35;36;56"},
		{"selected cursor", true, true, true, "48;2;48;52;80"},
		{"selected unfocused", true, true, false, "48;2;35;36;56"},
	}
	check := func(line, bg string) {
		t.Helper()
		if strings.Contains(line, "▌") || ansi.StringWidthWc(line) != 38 {
			t.Fatalf("selection marker or wrong width: %q", line)
		}
		// Every visible span, including indentation and trailing padding, carries the background.
		for _, span := range strings.Split(line, "\x1b[0m") {
			if ansi.Strip(span) != "" && bg != "" && !strings.Contains(span, bg) {
				t.Fatalf("unpainted cells in %q (want %s)", span, bg)
			}
		}
		if bg == "" && strings.Contains(line, "48;2;") {
			t.Fatalf("unselected row has a background: %q", line)
		}
		fmt.Fprintln(&snapshot, strconv.Quote(line))
	}
	for _, mode := range []string{"nerd", "compat"} {
		SetIcons(mode)
		for _, state := range states {
			for _, kind := range []colKind{cInactive, cActive, cStaging} {
				c := column{kind: kind, items: []item{{id: "selected", name: "中文名称 Long name for wrapping across several lines", icon: ic.workshop, pos: 12, unsaved: true}}, view: []int{0}, cursor: -1}
				if state.cursor {
					c.cursor = 0
				}
				m.marked = map[string]bool{"selected": state.selected}
				for _, wrapName := range []bool{false, true} {
					fmt.Fprintf(&snapshot, "%s %s column=%d wrap=%t\n", mode, state.name, kind, wrapName)
					lines := m.drawRowLines(&c, 0, state.focused, 38, wrapName)
					if wrapName && len(lines) < 2 {
						t.Fatal("name did not wrap")
					}
					for _, line := range lines {
						check(line, state.bg)
					}
					if got := strings.Contains(ansi.Strip(lines[0]), markSel); got != (state.cursor && state.focused) {
						t.Fatalf("cursor marker: %q", lines[0])
					}
				}
			}
			m.openLibrary()
			libAt(t, m, "Alpha")
			i := m.libr.cursor
			m.libr.marked = map[string]bool{}
			m.libToggleSel()
			if !state.selected {
				m.libr.marked = map[string]bool{}
			}
			if !state.cursor {
				m.libr.cursor = -1
			}
			m.libr.detail = !state.focused
			fmt.Fprintf(&snapshot, "%s %s library\n", mode, state.name)
			check(m.libRowLine(i, 38, 12), state.bg)
		}
	}
	// Normalize line endings only; preserve ANSI colors and cell padding.
	golden.RequireEqual(t, []byte(snapshot.String()))
}

// The position input's caret is independent of row selection (16 §3.6).
func TestPositionInputCaret(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	press(m, "m")
	typeIn(m, "harm")
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "harm▌") {
		t.Fatalf("position input lost its caret:\n%s", screen)
	}
}
