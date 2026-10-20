package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// pickFixture: the fixture plus N1 … N9, not enabled (Disabled: Beta, N1 … N9, 中文).
func pickFixture(t *testing.T) (*testkit.Env, *model) {
	e, _ := fixture(t)
	for i := 1; i <= 9; i++ {
		e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprintf("N%d", i)), testkit.Sample(fmt.Sprintf("tester.n%d", i), fmt.Sprintf("N%d", i)))
	}
	e.MustRun("scan")
	m := direct(t, e, 160, 40)
	press(m, "h")
	press(m, "g")
	press(m, "ctrl+down") // N1 (nothing picked yet: an ordinary move)
	if m.current().name != "N1" {
		t.Fatalf("cursor on %q", m.current().name)
	}
	return e, m
}

// T11 (06 §12.6): shift picks 5, ctrl picks 2 more, → into Enabled, ↑ three times, enter:
// the 7 keep their order at the right place, and one u undoes all of it.
func TestT11PickMoveUndo(t *testing.T) {
	e, m := pickFixture(t)
	for i := 0; i < 4; i++ {
		press(m, "shift+down") // N1 … N5
	}
	press(m, "ctrl+down") // N6
	press(m, "ctrl+down") // N7
	press(m, "ctrl+space")
	press(m, "ctrl+down") // N8
	press(m, "ctrl+down") // N9
	press(m, "ctrl+space")
	if got := pickedNames(m); got != "N1 N2 N3 N4 N5 N7 N9" {
		t.Fatalf("picked %q", got)
	}
	if got := activeNames(t, e); got != "Core, Alpha, Harmony" {
		t.Fatalf("picking changed the list: %s", got)
	}
	// the hint is on screen, beside the picks
	screen := ansi.Strip(m.View())
	if !strings.Contains(screen, "picked 7") || !strings.Contains(screen, "move to Enabled") {
		t.Fatalf("no hint:\n%s", screen)
	}
	press(m, "right") // into Enabled: nothing constrains them, so at the end (4 … 10)
	if m.focus != cActive || !strings.Contains(ansi.Strip(m.View()), "Enabled #4–10") {
		t.Fatalf("after →: focus %d\n%s", m.focus, ansi.Strip(m.View()))
	}
	for i := 0; i < 3; i++ {
		press(m, "up")
	}
	press(m, "enter")
	want := "N1, N2, N3, N4, N5, N7, N9, Core, Alpha, Harmony"
	if got := activeNames(t, e); got != want {
		t.Fatalf("after ↑↑↑ enter:\n got %s\nwant %s", got, want)
	}
	if m.picking() || m.current().name != "N1" || m.focus != cActive {
		t.Fatalf("after enter: picking %v, cursor %q", m.picking(), m.current().name)
	}
	press(m, "u")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony" {
		t.Fatalf("one u: %s", got)
	}
}

// T12: esc while picking puts everything back as it was, without an undo step.
func TestT12PickCancel(t *testing.T) {
	e, m := pickFixture(t)
	steps := e.App.Store.StepCount(m.st.Ix.Env.ID, "undo")
	press(m, "shift+down") // N1 N2
	press(m, "right")      // into Enabled
	press(m, "up")
	press(m, "right") // into Staging
	if len(m.cols[cStaging].items) != 2 {
		t.Fatalf("staging %d", len(m.cols[cStaging].items))
	}
	press(m, "esc")
	if got := activeNames(t, e); got != "Core, Alpha, Harmony" || len(m.cols[cStaging].items) != 0 {
		t.Fatalf("after esc: %s, staging %d", got, len(m.cols[cStaging].items))
	}
	if n := e.App.Store.StepCount(m.st.Ix.Env.ID, "undo"); n != steps {
		t.Fatalf("undo steps %d → %d", steps, n)
	}
	if m.picking() || m.focus != cInactive || m.current().name != "N1" {
		t.Fatalf("after esc: picking %v, focus %d, cursor %q", m.picking(), m.focus, m.current().name)
	}
	// ↑↓ outside Enabled do not move anything and say so
	press(m, "space")
	press(m, "up")
	if !strings.Contains(m.status, "Enabled only") {
		t.Fatalf("↑ in Disabled: %q", m.status)
	}
	press(m, "esc")
}
