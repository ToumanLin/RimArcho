package cmds_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// TestHarmonyOutsideList: harmony --mod answers for a Mod outside the current list, from an
// earlier analysis of that copy when there is one, otherwise from its assemblies read now; the
// shared view names its Mods (05 §5.3, 14 §2). The window's details pane (--list-only) shows
// only the stored facts, muted under where they came from (16 §4.1).
func TestHarmonyOutsideList(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("disable", ref(e, "Mod C"))
	e.MustRun("analyze")
	// The pane never reads assemblies: a Mod never analysed stays "not analysed".
	if out := e.MustRun("harmony", "--mod", ref(e, "Mod C"), "--list-only"); !strings.Contains(out, "harmony: not analysed (not in the current list)") {
		t.Fatalf("harmony --list-only, never analysed:\n%s", out)
	}
	out := e.MustRun("harmony", "--mod", ref(e, "Mod C"))
	if !strings.Contains(out, "scope: not in the current list: its assemblies read now for 1.6, not stored") ||
		!strings.Contains(out, "Verse.Pawn.Tick") || strings.Contains(out, "not analysed") {
		t.Fatalf("harmony --mod outside the list, never analysed:\n%s", out)
	}
	// Mod C in the list once, then out again: the stored facts of that copy answer.
	e.MustRun("enable", ref(e, "Mod C"))
	e.MustRun("analyze")
	e.MustRun("plan", "new", "Raid night", "--from", "current")
	e.MustRun("disable", ref(e, "Mod C"))
	out = e.MustRun("harmony", "--mod", ref(e, "Mod C"), "--shared")
	if !strings.Contains(out, "scope: not in the current list: from an earlier analysis of this copy for 1.6 (lists with it: plan:Raid night)") ||
		!strings.Contains(out, "Verse.Pawn.Tick") || !strings.Contains(out, "Mod C") || strings.Contains(out, "#?") {
		t.Fatalf("harmony --mod outside the list, analysed before:\n%s", out)
	}
	if out := e.MustRun("harmony"); strings.Contains(out, "Mod C") {
		t.Fatalf("shared targets list a Mod outside the list:\n%s", out)
	}
	if out := e.MustRun("harmony", "--mod", ref(e, "Mod C"), "--list-only"); !strings.Contains(out, "from an earlier analysis of this copy") || !strings.Contains(out, "Verse.Pawn.Tick") {
		t.Fatalf("harmony --list-only, analysed before:\n%s", out)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	harmony := func(name string) view.Section {
		for _, sec := range view.Sections(e.App, st.Ix, modNamed(t, st, name)) {
			if sec.Title == "Harmony" {
				return sec
			}
		}
		t.Fatalf("no Harmony section for %s", name)
		return view.Section{}
	}
	text := func(sec view.Section) string { return strings.Join(sec.Lines, "\n") }
	if sec := harmony("Mod C"); !sec.Muted || sec.Count != -1 || sec.Note != "from an earlier analysis in Raid night" ||
		strings.Contains(text(sec), "scope:") || !strings.Contains(text(sec), "Verse.Pawn.Tick") {
		t.Fatalf("pane: a Mod outside the list analysed before: %+v", sec)
	}
	// A Mod in the list is unchanged: not muted, no note, its count.
	if sec := harmony("Mod A"); sec.Muted || sec.Note != "" || sec.Count < 1 {
		t.Fatalf("pane: a Mod in the list: %+v", sec)
	}
}

func modNamed(t *testing.T, st *app.LoadoutState, name string) *store.Mod {
	t.Helper()
	for _, m := range st.Ix.Mods {
		if m.DisplayName() == name {
			return m
		}
	}
	t.Fatalf("no mod %s", name)
	return nil
}
