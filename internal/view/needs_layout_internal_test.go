package view

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Weak order preferences and cycles merge into the item that sorts (06 §5); without one
// they are hints of their own.
func TestNeedsSortAlso(t *testing.T) {
	t.Parallel()
	build := func(withHost bool) []Need {
		var items []Need
		if withHost {
			items = append(items, Need{Group: GroupList, Section: SectionBreak, Counted: true, code: "official.order", Text: "Core first", Actions: []Action{sortAction()}})
		}
		items = append(items,
			Need{Group: GroupList, Section: SectionHint, code: "order.violated", Text: "2 weaker", Parts: []string{"a", "b"}, Dim: true, Actions: []Action{sortAction(), {Kind: ActFold, Key: "f"}}},
			Need{Group: GroupList, Section: SectionBreak, code: "order.cycle", Text: "cycle", Mods: []string{"x", "y", "z"}, Why: "requirements set aside by sorting: Q",
				Actions: []Action{{Kind: ActList, Key: "enter", Label: "see which", Lines: []string{"x after y"}}}},
		)
		for i := range items {
			identifyNeed(&items[i])
		}
		mergeIntoSort(items)
		return items
	}
	items := build(true)
	host, weak, cycle := items[0], items[1], items[2]
	if weak.Into != host.ID || cycle.Into != host.ID {
		t.Fatalf("into: weak %q cycle %q host %q", weak.Into, cycle.Into, host.ID)
	}
	if len(host.Also) != 2 || !strings.Contains(host.Also[0], "2 weaker order preferences") || !strings.Contains(host.Also[1], "cycle of 3 mods") || !strings.Contains(host.Also[1], "Q") {
		t.Fatalf("also: %q", host.Also)
	}
	i := slices.IndexFunc(host.Actions, func(a Action) bool { return a.Kind == ActList })
	if i < 0 || host.Actions[i].Key != "v" || host.Actions[i].ID == "" || host.Actions[i].ID == cycle.Actions[0].ID {
		t.Fatalf("the cycle's list on the host: %+v", host.Actions)
	}
	items = build(false)
	for _, it := range items {
		if it.Into != "" || it.Section != SectionHint {
			t.Fatalf("no host: %+v", it)
		}
	}
}

// The last run's error groups and the left column's items about the same mod point at
// each other (06 §5).
func TestNeedsRelated(t *testing.T) {
	t.Parallel()
	items := []Need{
		{ID: "need:dep", Group: GroupDeps, Section: SectionBreak, code: "dep.inactive", Title: "Mech needs VEF, but it is not enabled", RequiredBy: []ModOwner{{ID: "m1"}, {ID: "m3"}}},
		{ID: "need:hint", Group: GroupConflict, Section: SectionHint, code: "mod.known-error", Text: "Furniture often errs", Mods: []string{"m1"}},
		{ID: "need:other", Group: GroupList, Section: SectionBreak, code: "official.order", Text: "Core first", Mods: []string{"m1"}},
		{ID: "need:log1", Group: GroupLastRun, Section: SectionRun, Title: "Furniture — 12 errors", Mods: []string{"m1"}},
		{ID: "need:log2", Group: GroupLastRun, Section: SectionRun, Title: "Other — 1 error", Mods: []string{"m2"}},
		{ID: "need:old", Group: GroupLastRun, Section: SectionRun, Text: "earlier log", Dim: true},
	}
	relateNeeds(items)
	dep, hint, other, log1, log2 := items[0], items[1], items[2], items[3], items[4]
	if len(dep.Related) != 1 || dep.Related[0].ID != "need:log1" || dep.Related[0].Text != "in the last run: Furniture — 12 errors" {
		t.Fatalf("dependency: %+v", dep.Related)
	}
	if len(hint.Related) != 1 || hint.Related[0].ID != "need:log1" {
		t.Fatalf("hint: %+v", hint.Related)
	}
	if len(other.Related) != 0 || len(log2.Related) != 0 {
		t.Fatalf("unrelated: %+v %+v", other.Related, log2.Related)
	}
	if len(log1.Related) != 2 || log1.Related[0].Text != "related: Mech needs VEF, but it is not enabled" || log1.Related[1].ID != "need:hint" {
		t.Fatalf("log: %+v", log1.Related)
	}
}

// What other pages handle is counted into one line each (06 §2.4); the current game's
// links come first.
func TestNeedsElsewhereAndLinks(t *testing.T) {
	t.Parallel()
	items := []Need{
		{Section: SectionElsewhere, code: "mod.hidden"}, {Section: SectionElsewhere, code: "mod.pinned"},
		{Section: SectionElsewhere, code: "texture.stale"}, {Section: SectionBreak, code: "official.order"},
	}
	got := elsewhere(items)
	if len(got) != 2 || got[0].Page != "library" || got[0].Count != 2 || !strings.Contains(got[0].Query, "state:hidden") ||
		got[1].Page != "textures" || got[1].Count != 1 || got[1].Text != "1 mods' converted textures are out of date" {
		t.Fatalf("elsewhere: %+v", got)
	}
	cur, other := filepath.Join(t.TempDir(), "Game"), filepath.Join(t.TempDir(), "Other")
	links := []Need{{LinkGroups: []LinkGroup{{Game: "other", Path: other}, {Game: "cur", Path: cur}},
		Actions: []Action{{Kind: ActRepairLinks, LinkGroups: []LinkGroup{{Path: other}, {Path: cur}}}}}}
	currentLinksFirst(links, cur)
	if g := links[0].LinkGroups; g[0].Game != "cur" || !g[0].Current || g[1].Current {
		t.Fatalf("groups: %+v", g)
	}
	if g := links[0].Actions[0].LinkGroups; !g[0].Current || g[0].Path != cur {
		t.Fatalf("the repair's groups: %+v", g)
	}
}
