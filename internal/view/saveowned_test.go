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
	if v.Head != "Kurin is not installed" || v.Consequence != want {
		t.Fatalf("not installed: %q / %q", v.Head, v.Consequence)
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
		for _, en := range []string{"colonist", "faction", "class (", "uses of", "inferred", "not checked", "the game drops", "malformed", "XML breaks", "missing", "nothing in"} {
			if strings.Contains(v.Head+v.Consequence, en) {
				t.Errorf("%s: untranslated %q in %q / %q", is.Kind, en, v.Head, v.Consequence)
			}
		}
	}
}
