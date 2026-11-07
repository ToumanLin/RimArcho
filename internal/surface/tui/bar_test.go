package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
	"github.com/charmbracelet/x/ansi"
)

func viewNames(m *model, k colKind) []string {
	var out []string
	c := &m.cols[k]
	for _, vi := range c.view {
		out = append(out, c.items[vi].name)
	}
	return out
}

// 16 §2.5: `\` opens the bar's groups; ↑↓ a group, typing finds a value, enter keeps it, x
// leaves it out, esc closes; the filter holds for Not enabled and Enabled together.
func TestBarGroups(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	press(m, `\`)
	if !m.bar.open || len(m.bar.groups) == 0 {
		t.Fatalf("open %v, groups %d", m.bar.open, len(m.bar.groups))
	}
	for m.bar.groups[m.bar.grp].Key != "source" {
		press(m, "down")
	}
	typeIn(m, "work")
	press(m, "enter")
	if !slices.Equal(m.bar.filters, []string{"source:workshop"}) {
		t.Fatalf("filters %v", m.bar.filters)
	}
	if got := strings.Join(viewNames(m, cActive), ","); got != "Alpha,Harmony" {
		t.Fatalf("enabled %q", got)
	}
	if got := len(m.cols[cInactive].view); got != 0 {
		t.Fatalf("disabled: %d rows (none of them is from the Workshop)", got)
	}
	bar := ansi.Strip(strings.Join(m.barView(160), "\n"))
	if !strings.Contains(bar, "[") || !strings.Contains(bar, "workshop") {
		t.Fatalf("bar %q", bar)
	}
	press(m, "x") // the same value, left out now
	if !slices.Equal(m.bar.filters, []string{"-source:workshop"}) || strings.Join(viewNames(m, cActive), ",") != "Core" {
		t.Fatalf("exclude: %v %v", m.bar.filters, viewNames(m, cActive))
	}
	press(m, "esc") // the find text
	press(m, "esc") // the groups
	if m.bar.open || !m.bar.active() {
		t.Fatalf("esc: open %v, filters %v", m.bar.open, m.bar.filters)
	}
	press(m, "esc") // the filters
	if m.bar.active() || len(m.cols[cActive].view) != 3 {
		t.Fatalf("esc cleared: %v %d", m.bar.filters, len(m.cols[cActive].view))
	}
	// the version group marks the game's version
	press(m, `\`)
	for _, g := range m.bar.groups {
		if g.Key == "version" && (len(g.Values) == 0 || !g.Values[0].Cur) {
			t.Fatalf("version group %+v", g.Values)
		}
	}
}

func TestListsBarKeyboard(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	e.MustRun("plan", "new", "Other colony", "--from", "current")
	m := direct(t, e, 160, 40)
	press(m, `\`)
	if len(m.bar.groups) == 0 || m.bar.groups[0].Key != "lists" {
		t.Fatalf("first group: %+v", m.bar.groups)
	}
	press(m, "enter")
	if !slices.Equal(m.bar.filters, []string{"lists:Other colony"}) || len(m.cols[cActive].view) != 3 {
		t.Fatalf("list include: %v, %v", m.bar.filters, viewNames(m, cActive))
	}
	press(m, "x")
	if !slices.Equal(m.bar.filters, []string{"-lists:Other colony"}) || len(m.cols[cActive].view) != 0 {
		t.Fatalf("list exclusion: %v, %v", m.bar.filters, viewNames(m, cActive))
	}
	text := valText(view.LibChoice{Filter: "lists:Main colony", Label: "Main colony", Cur: true, Count: 3})
	if !strings.Contains(text, T("current list")) || strings.Contains(text, T("this game")) {
		t.Fatalf("current-list marker: %s", text)
	}
}

// 16 §3.5: J / K while the bar filters move past the next shown row; space and m keep the
// full order's places.
func TestBarMoves(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	press(m, "2")
	press(m, "/")
	typeIn(m, "-alp") // shows Core (1) and Harmony (3)
	press(m, "enter")
	if got := strings.Join(viewNames(m, cActive), ","); got != "Core,Harmony" {
		t.Fatalf("shown %q", got)
	}
	press(m, "J") // Core past Harmony
	if got := activeNames(t, e); got != "Alpha, Harmony, Core" {
		t.Fatalf("J with a filter: %s (status %q)", got, m.status)
	}
	if m.current() == nil || m.current().name != "Core" {
		t.Fatalf("cursor %v", m.current())
	}
	press(m, "J") // the last shown row: nowhere to go
	if got := activeNames(t, e); got != "Alpha, Harmony, Core" || !strings.Contains(m.status, "edge") {
		t.Fatalf("J at the edge: %s %q", got, m.status)
	}
	press(m, "K") // back before Harmony
	if got := activeNames(t, e); got != "Alpha, Core, Harmony" {
		t.Fatalf("K with a filter: %s", got)
	}
	// enabling from a filtered Not enabled: the place the order wants, in the full list
	press(m, "esc")
	press(m, "/")
	typeIn(m, "beta")
	press(m, "enter")
	press(m, "1")
	press(m, "space")
	if got := activeNames(t, e); got != "Alpha, Core, Harmony, Beta" || !strings.Contains(m.status, "position 4") {
		t.Fatalf("enable: %s %q", got, m.status)
	}
	// m: the position input counts the full list
	press(m, "2")
	if m.current() == nil || m.current().name != "Beta" {
		t.Fatalf("Beta is shown in Enabled: %v", viewNames(m, cActive))
	}
	press(m, "m")
	typeIn(m, "1")
	press(m, "enter")
	if got := activeNames(t, e); got != "Beta, Alpha, Core, Harmony" {
		t.Fatalf("m 1 with a filter: %s", got)
	}
}
