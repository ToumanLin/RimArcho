package view

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// The rows of Can it load? that come from the full read (10 §4.3, §4.6).
func TestSaveIssueViewOwnership(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	before := Lang()
	t.Cleanup(func() { SetLang(before) })
	SetLang("en")
	kurin := app.SaveMod{PackageID: "tester.kurin", Name: "Kurin", SteamID: "3000000005"}
	owns := &app.SaveOwned{Colonists: []app.OwnedRace{{Race: "Kurin_Race", Alive: 2, Dead: 1}}, Factions: []string{"Kurin Clan"},
		Classes: []string{"ModA.CompBroken"}, Defs: []app.OwnedDefs{{Type: "ThingDef", Defs: 2, Uses: 3}}, Inferred: true, Among: 1}
	s := &app.Save{}
	v := issueView(app.SaveIssue{Kind: app.IssueNotInstalled, Level: app.LevelError, Mods: []app.SaveMod{kurin}, Owns: owns}, s)
	want := "loading without it loses 3 colonists of race Kurin_Race (1 dead), 1 faction (Kurin Clan), 1 class (ModA.CompBroken), 3 uses of ThingDef" +
		"; inferred: nothing installed provides them; they may belong to the other 1 Mod without facts instead"
	if v.Head != "Kurin" || v.Consequence != want || v.Group != SaveGroupMods {
		t.Fatalf("not installed: %q / %q", v.Head, v.Consequence)
	}
	if d := v.Details; d == nil || len(d.Lost) != 4 || d.Lost[3].Kind != "ThingDef" || d.Lost[2].Kind != "classes" || d.Lost[2].Names[0] != "ModA.CompBroken" {
		t.Fatalf("details: %+v", v.Details)
	}
	// Two Mods the database knows nothing of share what nothing installed provides: one
	// row, counted once, with the Def names in its details.
	pool := &app.SaveOwned{Defs: []app.OwnedDefs{{Defs: 2, Uses: 630, Names: []string{"VCE_Soup1", "VCE_Soup2"}}}, Inferred: true, Among: 1, NoFacts: true}
	together := &app.SaveOwned{Defs: pool.Defs, Inferred: true}
	soup := app.SaveMod{PackageID: "vce.soup", Name: "VCE Soup"}
	head := app.SaveMod{PackageID: "cut.head", Name: "Cut Head"}
	g := goneView([]app.SaveIssue{{Kind: app.IssueNotInstalled, Level: app.LevelWarning, Mods: []app.SaveMod{head}, Owns: pool},
		{Kind: app.IssueNotInstalled, Level: app.LevelWarning, Mods: []app.SaveMod{soup}, Owns: pool}}, together, s)
	if g.Head != "Cut Head, VCE Soup" || g.Consequence != "loading without them loses 630 Def uses; the database has no data on these 2 Mods" ||
		g.Details.Lost[0].Names[1] != "VCE_Soup2" || len(g.Fixes) != 0 {
		t.Fatalf("two unknown Mods: %+v", g)
	}
	// What could not be checked is said on an installed Mod's row.
	if v := issueView(app.SaveIssue{Kind: app.IssueDisabled, Mods: []app.SaveMod{kurin}, Owns: &app.SaveOwned{ClassesUnchecked: true}}, s); v.Head != "Kurin" ||
		v.Consequence != "nothing in the save is known to need it; what else it stored is removed when the save loads; assemblies of the save's list not analysed yet" {
		t.Fatalf("disabled, classes unchecked: %q / %q", v.Head, v.Consequence)
	}
	// Without the full read the sentence is the generic one.
	if v := issueView(app.SaveIssue{Kind: app.IssueDisabled, Mods: []app.SaveMod{kurin}}, s); v.Consequence != "what it stored in the save is removed when the save loads without it" {
		t.Fatalf("no full read: %q", v.Consequence)
	}
	v = issueView(app.SaveIssue{Kind: app.IssueClassMissing, Level: app.LevelError, Text: "A.B, A.C", Owns: &app.SaveOwned{Classes: []string{"A.B", "A.C"}}}, s)
	if v.Head != "2 types are missing" || !strings.HasSuffix(v.Consequence, "  (A.B, A.C)") || !strings.Contains(v.Consequence, "no installed Mod nor the game") {
		t.Fatalf("class missing: %q / %q", v.Head, v.Consequence)
	}
	v = issueView(app.SaveIssue{Kind: app.IssueMalformed, Level: app.LevelError, Text: "line 3 (byte 40): unexpected end element"}, s)
	if v.Head != "the file is malformed" || !strings.HasSuffix(v.Consequence, "(line 3 (byte 40): unexpected end element)") {
		t.Fatalf("malformed: %q / %q", v.Head, v.Consequence)
	}

	// Every sentence is translated.
	SetLang("zh")
	for _, is := range []app.SaveIssue{
		{Kind: app.IssueNotInstalled, Mods: []app.SaveMod{kurin}, Owns: owns},
		{Kind: app.IssueDisabled, Mods: []app.SaveMod{kurin}, Owns: &app.SaveOwned{DefsUnchecked: true, ClassesUnchecked: true}},
		{Kind: app.IssueClassMissing, Mods: []app.SaveMod{kurin}, Owns: &app.SaveOwned{Classes: []string{"A.B"}}},
		{Kind: app.IssueMalformed, Text: "x"},
	} {
		v := issueView(is, s)
		if v.Details != nil {
			for _, m := range v.Details.Mods {
				v.Consequence += " " + m.Note
			}
		}
		for _, en := range []string{"colonist", "faction", "class (", "uses of", "inferred", "not checked", "not analysed", "the game drops", "malformed", "XML breaks", "missing", "nothing in", "database", "Workshop"} {
			if strings.Contains(v.Head+v.Consequence, en) {
				t.Errorf("%s: untranslated %q in %q / %q", is.Kind, en, v.Head, v.Consequence)
			}
		}
	}
}
