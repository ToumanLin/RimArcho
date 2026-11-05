package app_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// ownersFixture: Core with the vanilla Defs the saves use, Alpha, and Kurin (a race, its
// faction, a hediff, an item and ModA.dll), all enabled and analysed; the game's own
// assembly indexed. A launch an hour ago matches later saves to the list.
func ownersFixture(t *testing.T) *testkit.Env {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Races.xml"),
		"<Defs><ThingDef><defName>Human</defName></ThingDef><FactionDef><defName>PlayerColony</defName></FactionDef><HediffDef><defName>Cut</defName></HediffDef></Defs>\n")
	copyFixtureDLL(t, e, "Assembly-CSharp.dll", filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", store.GameMainAssembly))
	dll, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), "ModA.dll"))
	if err != nil {
		t.Fatal(err)
	}
	e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("3000000005", testkit.Mod{PackageID: "tester.kurin", Name: "Kurin", Files: map[string]string{
		"Defs/Kurin.xml": "<Defs><ThingDef><defName>Kurin_Race</defName></ThingDef><FactionDef><defName>KurinClan</defName></FactionDef>" +
			"<HediffDef><defName>KurinTail</defName></HediffDef><ThingDef><defName>KurinTea</defName></ThingDef></Defs>\n",
		"Assemblies/ModA.dll": string(dll),
	}})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha", "tester.kurin")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.RecordLaunch(e.Now().Add(-time.Hour))
	return e
}

// kurinBody: two Kurin colonists, one dead in a corpse, a human with a Kurin hediff, two
// Kurin teas and a game component of ModA.
const kurinBody = `<maps><li><uniqueID>0</uniqueID><things>
<thing Class="Pawn"><def>Kurin_Race</def><id>Kurin_Race1</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /></thing>
<thing Class="Pawn"><def>Kurin_Race</def><id>Kurin_Race2</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /></thing>
<thing Class="ThingWithComps"><def>Corpse_Kurin_Race</def><id>Corpse_Kurin_Race3</id><innerContainer><innerList><li>
 <def>Kurin_Race</def><id>Kurin_Race4</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /><healthTracker><healthState>Dead</healthState></healthTracker></li></innerList></innerContainer></thing>
<thing Class="Pawn"><def>Human</def><id>Human5</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story />
 <healthTracker><hediffSet><hediffs><li><def>Cut</def></li><li><def>KurinTail</def></li></hediffs></hediffSet></healthTracker></thing>
<thing Class="ThingWithComps"><def>KurinTea</def><id>KurinTea6</id></thing>
<thing Class="ThingWithComps"><def>KurinTea</def><id>KurinTea7</id></thing>
</things></li></maps>
<components><li Class="ModA.CompBroken" /></components>`

// writeOwnersSave writes a save with the Kurin faction beside the player's.
func writeOwnersSave(t *testing.T, e *testkit.Env, file string, spec testkit.SaveSpec) {
	t.Helper()
	x := testkit.SaveXML(spec)
	x = strings.Replace(x, "</allFactions>", "<li><def>KurinClan</def><name>Kurin Clan</name><loadID>2</loadID></li></allFactions>", 1)
	p := filepath.Join(e.SavesDir(), file)
	e.WriteFile(p, x)
	at := e.Now().Add(-time.Minute)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

func checkNamed(t testing.TB, e *testkit.Env, name string) *app.SaveCheck {
	t.Helper()
	s, _, err := e.App.FindSave(name)
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.App.CheckSave(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func issueKinds(c *app.SaveCheck) []string {
	var out []string
	for _, is := range c.Issues {
		k := is.Level + " " + is.Kind
		for _, m := range is.Mods {
			k += " " + m.PackageID
		}
		if is.Text != "" {
			k += " [" + is.Text + "]"
		}
		out = append(out, k)
	}
	return out
}

// What a Mod owns in a save (10 §4.6) and the rows of Can it load? that need the full read
// (10 §4.3): the same save while Kurin is enabled, disabled and removed; a class no Mod
// defines; malformed XML (SV14).
func TestCheckSaveOwnership(t *testing.T) {
	t.Parallel()
	e := ownersFixture(t)
	mods := []string{"ludeon.rimworld", "tester.alpha", "tester.kurin"}
	spec := testkit.SaveSpec{Mods: mods, Names: []string{"Core", "Alpha", "Kurin"}, SteamIDs: []string{"", "3000000002", "3000000005"},
		Faction: "New Arrivals", Body: kurinBody}
	writeOwnersSave(t, e, "Foxes.rws", spec)
	odd := spec
	odd.Body = `<components><li Class="Gone.Thing" /><li Class="Pawn" /><li Class="System.Collections.Generic.List` + "`" + `1[Verse.Pawn]" /></components>`
	e.WriteSave("Odd.rws", odd)
	bad := spec
	bad.Body = "<oops></nope>"
	e.WriteSave("Bad.rws", bad)
	cut := spec
	cut.CutOff = true
	e.WriteSave("Cut.rws", cut)

	// Without the full read nothing changes: the save checks from its head.
	if c := checkNamed(t, e, "Foxes"); c.Level != "" || len(c.Issues) != 0 {
		t.Fatalf("before indexing: %v", issueKinds(c))
	}
	if err := e.App.RunSavesIndex("", nil, nil); err != nil {
		t.Fatal(err)
	}

	// Everything installed and enabled: the classes resolve (ModA's type, the game's Pawn,
	// a generic of game and runtime types) except one no assembly defines.
	if c := checkNamed(t, e, "Foxes"); c.Level != "" || len(c.Issues) != 0 {
		t.Fatalf("all enabled: %v", issueKinds(c))
	}
	c := checkNamed(t, e, "Odd")
	if got, want := issueKinds(c), []string{"error class-missing [Gone.Thing]"}; !slices.Equal(got, want) {
		t.Fatalf("odd classes: %q, want %q", got, want)
	}
	if c.Level != app.LevelError || c.Issues[0].Owns == nil || !slices.Equal(c.Issues[0].Owns.Classes, []string{"Gone.Thing"}) {
		t.Fatalf("class-missing: level %q owns %+v", c.Level, c.Issues[0].Owns)
	}

	// Malformed XML is an error that says where reading stopped; a file that only ends
	// early keeps its one cut-off row.
	c = checkNamed(t, e, "Bad")
	if len(c.Issues) != 1 || c.Issues[0].Kind != app.IssueMalformed || c.Level != app.LevelError || !strings.HasPrefix(c.Issues[0].Text, "line ") ||
		c.Quick == nil || c.Quick.Action != app.FixRecycle {
		t.Fatalf("malformed: %v quick %+v", issueKinds(c), c.Quick)
	}
	c = checkNamed(t, e, "Cut")
	if got := issueKinds(c); len(got) != 1 || c.Issues[0].Kind != app.IssueCutOff {
		t.Fatalf("cut off: %q", got)
	}

	// Kurin disabled: installed, so its Def facts and its kept type facts say exactly what
	// it owns.
	e.MustRun("disable", "tester.kurin")
	c = checkNamed(t, e, "Foxes")
	if len(c.Issues) != 1 || c.Issues[0].Kind != app.IssueDisabled || c.Issues[0].Level != app.LevelError || c.Level != app.LevelError {
		t.Fatalf("disabled: %v", issueKinds(c))
	}
	want := &app.SaveOwned{
		Colonists: []app.OwnedRace{{Race: "Kurin_Race", Alive: 2, Dead: 1}},
		Factions:  []string{"Kurin Clan"},
		Classes:   []string{"ModA.CompBroken"},
		Defs:      []app.OwnedDefs{{Type: "ThingDef", Defs: 2, Uses: 3}, {Type: "HediffDef", Defs: 1, Uses: 1}},
	}
	if got := c.Issues[0].Owns; !reflect.DeepEqual(got, want) {
		t.Fatalf("disabled Kurin owns %+v\nwant %+v", got, want)
	}
	if got := strings.Join(want.Parts(), "; "); got != "3 colonists of race Kurin_Race (1 dead); 1 faction (Kurin Clan); 1 class (ModA.CompBroken); 3 things; 1 hediff" {
		t.Fatalf("parts %q", got)
	}
	if f := c.Issues[0].Fixes; len(f) != 1 || f[0].Action != app.FixEnable {
		t.Fatalf("disabled fixes %+v", f)
	}

	// Kurin removed from disk: its tombstone keeps the facts of its last copy (02 §5.5), so
	// what it owns is still known, not inferred.
	e.Remove(filepath.Join(e.Workshop, "3000000005"))
	e.Unsubscribe("3000000005")
	e.MustRun("scan")
	e.MustRun("analyze")
	c = checkNamed(t, e, "Foxes")
	if len(c.Issues) != 1 || c.Issues[0].Kind != app.IssueNotInstalled || c.Issues[0].Level != app.LevelError {
		t.Fatalf("removed: %v", issueKinds(c))
	}
	if got := c.Issues[0].Owns; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed Kurin owns %+v\nwant %+v", got, want)
	}
	if f := c.Issues[0].Fixes; len(f) != 1 || f[0].Action != app.FixSubscribe {
		t.Fatalf("not-installed fixes %+v", f)
	}
	out, _ := e.Run("saves", "show", "Foxes")
	if !strings.Contains(out, "consequence: loading without it loses 3 colonists of race Kurin_Race (1 dead), 1 faction (Kurin Clan), 1 class (ModA.CompBroken), 3 things, 1 hediff\n") ||
		!strings.Contains(out, "[will not load cleanly]") {
		t.Fatalf("saves show:\n%s", out)
	}
	// Kurin's facts say the vanished class is not its: the class keeps its own row, and
	// Kurin, owning nothing in that save, is a warning.
	c = checkNamed(t, e, "Odd")
	if got, want := issueKinds(c), []string{"error class-missing [Gone.Thing]", "warning not-installed tester.kurin"}; !slices.Equal(got, want) {
		t.Fatalf("odd after removal: %q, want %q", got, want)
	}
	if o := c.Issues[1].Owns; o == nil || !o.Empty() || o.Inferred {
		t.Fatalf("kurin in odd owns %+v", o)
	}
	if out, _ := e.Run("saves", "show", "Odd"); !strings.Contains(out, "error class-missing  Gone.Thing\n") ||
		!strings.Contains(out, "consequence: nothing in the save is known to need it; what else it stored is lost on load") {
		t.Fatalf("saves show odd:\n%s", out)
	}

	// Two Mods never installed: no facts anywhere, so the race, the faction, the class and
	// the Defs nothing installed defines are inferred to be theirs, shared between them.
	// Kurin's known content stays its own; implied Defs (a corpse) resolve by their race.
	never := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.alpha", "tester.zorr", "tester.yip"}, SteamIDs: []string{"", "3000000002", "3000000008", ""},
		Faction: "New Arrivals", Body: `<maps><li><uniqueID>0</uniqueID><things>
<thing Class="Pawn"><def>Zorr_Race</def><id>Zorr_Race1</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /></thing>
<thing Class="Pawn"><def>Human</def><id>Human2</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><story /></thing>
<thing Class="ThingWithComps"><def>Corpse_Human</def><id>Corpse_Human3</id></thing>
<thing Class="ThingWithComps"><def>ZorrHat</def><id>ZorrHat4</id></thing>
</things></li></maps><components><li Class="ZorrWorld" /></components>`}
	e.WriteSave("Zorr.rws", never)
	if err := e.App.RunSavesIndex("", []string{"Zorr"}, nil); err != nil {
		t.Fatal(err)
	}
	c = checkNamed(t, e, "Zorr")
	if got, want := issueKinds(c), []string{"error not-installed tester.zorr", "error not-installed tester.yip"}; !slices.Equal(got, want) {
		t.Fatalf("never installed: %q, want %q", got, want)
	}
	for _, is := range c.Issues {
		o := is.Owns
		if o == nil || !o.Inferred || o.Among != 1 || !reflect.DeepEqual(o.Colonists, []app.OwnedRace{{Race: "Zorr_Race", Alive: 1}}) ||
			!slices.Equal(o.Classes, []string{"ZorrWorld"}) || !reflect.DeepEqual(o.Defs, []app.OwnedDefs{{Defs: 1, Uses: 1}}) {
			t.Fatalf("%s owns %+v", is.Mods[0].PackageID, o)
		}
	}
	if out, _ := e.Run("saves", "show", "Zorr"); !strings.Contains(out, "loses 1 colonist of race Zorr_Race, 1 class (ZorrWorld), 1 Def use "+
		"(inferred: nothing installed provides them; they may belong to the other 1 Mod without facts instead)") {
		t.Fatalf("saves show zorr:\n%s", out)
	}
}

// Without an analysis of the game version nothing can be checked: the rows are those of
// the head, as before the full read.
func TestCheckSaveOwnershipUnchecked(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("scan")
	e.RecordLaunch(e.Now().Add(-time.Hour))
	e.WriteSave("Lone.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.zorr"}, Faction: "Few",
		Body: `<components><li Class="ZorrWorld" /></components>`})
	if err := e.App.RunSavesIndex("", nil, nil); err != nil {
		t.Fatal(err)
	}
	c := checkNamed(t, e, "Lone")
	if got := issueKinds(c); !slices.Equal(got, []string{"warning not-installed tester.zorr"}) || c.Issues[0].Owns != nil {
		t.Fatalf("unchecked: %q owns %+v", got, c.Issues[0].Owns)
	}
}
