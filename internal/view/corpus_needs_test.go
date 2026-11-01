package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/known"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The log corpus in Can it run (06 §2.4, §5): an undeclared prerequisite with the dependencies,
// a known-bad pair with the conflicts, a mod's own known error as a dimmed note, each with
// the community logs chip and its measured share; weak order preferences folded, dimmed and not
// counted; the launch hint is the first LaunchChecks code with a counted item.
func TestCorpusNeeds(t *testing.T) {
	view.SetLang("en")
	defer known.SetForTesting(&known.Data{
		Cutoff:  "2040-03",
		Prereqs: []*known.Prereq{{Mod: "tester.user", Needs: "tester.lib", Logs: 40, Players: 30, Without: 50, Items: []string{"type Lib.Comp"}, Class: "xml-load"}},
		Combos: []*known.Combo{
			{Mods: []string{"tester.user", "tester.other"}, Class: "patch-failed", Templates: []string{"patch-failed|[User] Patch operation failed"}, Logs: 30, Players: 20, With: 40, Without: 100, WithoutErr: 2},
			{Mods: []string{"tester.user"}, Class: "exception", Templates: []string{"exception|Error in static constructor of User.Init"}, Logs: 90, Players: 60, With: 100},
		},
	})()
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	e.AddWorkshop("1001", testkit.Sample("tester.lib", "Lib"))
	e.AddWorkshop("1002", testkit.Sample("tester.user", "User"))
	other := testkit.Sample("tester.other", "Other")
	// loadAfter User, broken by the list below: a weak preference
	other.Files = map[string]string{"About/About.xml": strings.Replace(other.AboutXML(), "</ModMetaData>",
		"  <loadAfter><li>tester.user</li></loadAfter>\n</ModMetaData>", 1)}
	e.AddWorkshop("1003", other)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.other", "tester.user")
	e.AddEnv(false)
	e.MustRun("scan")
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := view.BuildNeeds(e.App, b.St, ps)
	find := func(sub string) *view.Need {
		for i := range n.Items {
			if strings.Contains(n.Items[i].Text, sub) {
				return &n.Items[i]
			}
		}
		t.Fatalf("no item %q in %+v", sub, n.Items)
		return nil
	}
	dep := find("seems to need Lib")
	if dep.Group != view.GroupDeps || dep.Counted || dep.Evidence == nil || dep.Evidence.Kind != view.EvCorpus ||
		!strings.Contains(dep.Evidence.Source, "40 of 50 (80%)") {
		t.Errorf("observed prerequisite: %+v %+v", dep, dep.Evidence)
	}
	pair := find("often produce errors together")
	if pair.Group != view.GroupConflict || pair.Counted || pair.Evidence == nil || pair.Evidence.Kind != view.EvCorpus ||
		!strings.Contains(pair.Evidence.Source, "30 of 40 (75%)") || !strings.Contains(pair.Evidence.Source, "2 of 100 (2%)") {
		t.Errorf("known-bad pair: %+v %+v", pair, pair.Evidence)
	}
	own := find("this is the mod's own problem")
	if own.Counted || !own.Dim || own.Section != view.SectionHint || !strings.Contains(own.Why, "not your list's") {
		t.Errorf("known error: %+v", own)
	}
	weak := find("weaker order preferences")
	if weak.Counted || !weak.Dim || len(weak.Parts) != 1 || !strings.Contains(weak.Parts[0], "Other should load after User") {
		t.Errorf("weak order: %+v", weak)
	}
	if n.Launch != nil {
		t.Errorf("launch hint: %+v", n.Launch)
	}
	// Corpus suggestions stay visible, including on rows, without becoming problems.
	if n.Count != 0 {
		t.Errorf("count %d, want 0: %+v", n.Count, n.Items)
	}
	for _, it := range []*view.Need{dep, pair, own, weak} {
		// worth knowing (06 §2.4): nothing sorts here, so the weak preferences are a hint too
		if it.Section != view.SectionHint || it.Into != "" {
			t.Errorf("not in Things to watch: %+v", it)
		}
		if it.Status != view.StatusHint || it.Evidence.Cutoff != "2040-03" || !strings.Contains(it.Evidence.Label, "2040-03") {
			t.Errorf("hint or cutoff: %+v", it)
		}
		for _, id := range it.Mods[:1] {
			if n.Status[id] != view.StatusHint || len(n.Reasons[id]) == 0 || n.Reasons[id][0].Evidence.Cutoff != "2040-03" {
				t.Errorf("missing hint badge: %s %+v", id, n.Reasons[id])
			}
		}
	}
	for _, m := range view.ProblemsOf(n).Mods {
		if m.Count != 0 || m.Severity != "info" {
			t.Errorf("hint counted as a problem: %+v", m)
		}
	}
}

// The pattern card of a log cluster (06 §2.10): community background  in the player's
// words; the CLI's English pattern section is left out of the text the interfaces show.
func TestPatternCard(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	e.AddEnv(false)
	e.MustRun("scan")
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "Player.log")
	log := "Mono path[0] = 'x'\nRimWorld 1.6.4633 rev1261\n\nCould not resolve cross-reference: No Verse.ThingDef named Lib_Gun found to give to Verse.ThingDefCountClass Verse.ThingDefCountClass\n"
	if err := os.WriteFile(path, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	c := view.PatternCardOf(e.App, b.St, path, "E1")
	if c == nil || c.Ref != "pattern:g:xref.def" || !strings.HasPrefix(c.Summary, "this kind of error is usually: ") ||
		len(c.Causes) == 0 || !strings.HasPrefix(c.Next, "try first: ") || len(c.Fixes) < 2 || !strings.Contains(c.Seen, "shared 1.6 logs") {
		t.Fatalf("card: %+v", c)
	}
	// a def that names a missing def: an error while loading, medium
	if c.Level != "medium" || c.LevelText != "severity medium" || !strings.Contains(c.Impact, "while loading") {
		t.Errorf("severity: %q %q %q", c.Level, c.LevelText, c.Impact)
	}
	lines := c.Lines()
	if !strings.HasPrefix(lines[0], "community background  ") {
		t.Errorf("card lines: %q", lines)
	}
	text := []string{"log:E1 xref-missing x1", "  message: m", "  pattern: pattern:g:xref.def  a def …", "    evidence: community", "    causes: x", "  known-error: pattern:m:x  …", "    repeat-uploads: y", "next:", "  archo x"}
	got := strings.Join(view.WithoutPatternBlock(text), "|")
	if got != "log:E1 xref-missing x1|  message: m|next:|  archo x" {
		t.Errorf("without pattern block: %s", got)
	}
}
