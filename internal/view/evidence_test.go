package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// G13 in part: every item sits in one of the four sections by its group, and the items of
// Will cause errors say where their claim comes from (06 §2.4).
func TestNeedsSectionsAndEvidence(t *testing.T) {
	t.Parallel()
	_, _, n := shellEnv(t)
	for g, want := range map[int]int{
		view.GroupList: view.SectionBreak, view.GroupDeps: view.SectionBreak, view.GroupConflict: view.SectionBreak,
		view.GroupLastRun: view.SectionRun, view.GroupThisRun: view.SectionRun,
		view.GroupConfirm: view.SectionConfirm, view.GroupHidden: view.SectionElsewhere,
	} {
		if got := view.SectionOf(g); got != want {
			t.Errorf("SectionOf(%d) = %d, want %d", g, got, want)
		}
	}
	if len(view.SectionTitles) != 5 {
		t.Fatalf("sections %v", view.SectionTitles)
	}
	var dep *view.Need
	for i, it := range n.Items {
		if it.Kind == "memory.capacity" {
			if it.Section != view.SectionHint {
				t.Errorf("unknown memory belongs in hints: %+v", it)
			}
			continue
		}
		if it.Section != view.SectionOf(it.Group) {
			t.Errorf("item %q: section %d, group %d", it.Text, it.Section, it.Group)
		}
		if it.Group == view.GroupDeps {
			dep = &n.Items[i]
		}
	}
	if dep == nil {
		t.Fatalf("no dependency item: %+v", n.Items)
	}
	// Alpha needs Harmony: the card's line names the dependency, the sentence the mod, and
	// the chip says the mod declares it.
	if dep.Title != "missing dependency: Harmony" || dep.Why != "Alpha needs it, but it is not enabled." {
		t.Fatalf("title %q why %q", dep.Title, dep.Why)
	}
	if e := dep.Evidence; e == nil || e.Kind != view.EvDeclared || e.Label != "declared by the mod" || e.Source != "About.xml modDependencies" {
		t.Fatalf("evidence %+v", dep.Evidence)
	}
	// The TUI's one sentence is unchanged.
	if dep.Text != "Alpha needs Harmony, but Harmony is not enabled" {
		t.Fatalf("text %q", dep.Text)
	}
}

// One mod's definitions with a missing parent are one item (06 §5's merge rule): the
// title counts them, the parts list each, and the count goes up by one.
func TestNeedsMergePerMod(t *testing.T) {
	t.Parallel()
	e, b, _ := shellEnv(t)
	var alpha string
	for _, m := range b.St.Ix.Mods {
		if m.DisplayName() == "Alpha" {
			alpha = m.ID
		}
	}
	if alpha == "" {
		t.Fatal("no Alpha")
	}
	parent := func(def string) *app.Problem {
		return &app.Problem{Finding: &finding.Finding{Code: "def.parent-missing", Severity: finding.Error, Subjects: []string{finding.Inst(alpha)},
			Claim:    "Verse.ThingDef " + def + ` inherits ParentName="BaseThing", which no loaded def has as its Name (after patches)`,
			Evidence: []finding.Evidence{{Source: finding.Inferred, Text: "x"}}}}
	}
	one := view.BuildNeeds(e.App, b.St, &app.ProblemSet{List: []*app.Problem{parent("A_Def")}})
	if len(one.Items) != 2 || one.Items[1].Kind != "memory.capacity" || one.Items[0].Fold || one.Items[0].Evidence != nil {
		t.Fatalf("one %+v", one.Items)
	}
	three := view.BuildNeeds(e.App, b.St, &app.ProblemSet{List: []*app.Problem{parent("A_Def"), parent("B_Def"), parent("C_Def")}})
	if len(three.Items) != 2 || three.Items[1].Kind != "memory.capacity" || three.Count != 1 {
		t.Fatalf("merged into %d items, count %d", len(three.Items), three.Count)
	}
	it := three.Items[0]
	if !it.Fold || len(it.Parts) != 3 || it.Title != "Alpha: 3 added things cannot load" || it.Evidence != nil || strings.Contains(it.Why, ". ") || !strings.Contains(it.Why, "16%") {
		t.Fatalf("merged %+v %+v", it, it.Evidence)
	}
	if last := it.Actions[len(it.Actions)-1]; last.Kind != view.ActFold {
		t.Fatalf("no fold action: %+v", it.Actions)
	}
}
