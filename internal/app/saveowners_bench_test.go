package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// BenchmarkSavesListFullRead times `archo saves list` over 100 synthetic saves (12 §4.3,
// 16 §10): read from the head only, then with every save read in full, so CheckSave
// attributes classes in every save and Defs in the half missing a Mod. Each save names
// 1,400 Defs (1,300 of Core's, like a real 1.6 save's 1,382, 18) and 25 classes. "cold"
// opens the data directory again for each run, as the CLI does; "warm" reuses it, as the
// GUI does.
func BenchmarkSavesListFullRead(b *testing.B) {
	e := testkit.New(b)
	var core strings.Builder
	core.WriteString("<Defs>")
	for i := range 1300 {
		fmt.Fprintf(&core, "<ThingDef><defName>Core%04d</defName></ThingDef>", i)
	}
	core.WriteString("<ThingDef><defName>Human</defName></ThingDef><FactionDef><defName>PlayerColony</defName></FactionDef></Defs>\n")
	e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Many.xml"), core.String())
	game, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), "Assembly-CSharp.dll"))
	if err != nil {
		b.Fatal(err)
	}
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", "Assembly-CSharp.dll"), string(game))
	dll, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), "ModA.dll"))
	if err != nil {
		b.Fatal(err)
	}
	mods := []string{"ludeon.rimworld"}
	for i := range 40 {
		m := testkit.Sample(fmt.Sprintf("tester.m%02d", i), fmt.Sprintf("M%02d", i))
		if i == 0 {
			m.Files["Assemblies/ModA.dll"] = string(dll)
		}
		e.AddWorkshop(fmt.Sprint(3100000000+i), m)
		mods = append(mods, m.PackageID)
	}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.RecordLaunch(e.Now().Add(-48 * time.Hour))
	for i := range 100 {
		colony := i / 10
		var body strings.Builder
		body.WriteString("<maps><li><uniqueID>0</uniqueID><things>")
		for d := range 1300 {
			fmt.Fprintf(&body, "<thing Class=\"ThingWithComps\"><def>Core%04d</def></thing>", (d+i)%1300)
		}
		for d := range 100 {
			fmt.Fprintf(&body, "<thing Class=\"ThingWithComps\"><def>m%02d_Thing</def></thing>", d%40) // Sample's Defs
			fmt.Fprintf(&body, "<thing><def>Colony%d_%d</def></thing>", colony, d)                     // no installed Mod defines these
		}
		body.WriteString("</things></li></maps><components>")
		for c := range 20 {
			fmt.Fprintf(&body, "<li Class=\"Colony%d.Comp%d\" />", colony, c)
		}
		body.WriteString(`<li Class="ModA.CompBroken" /><li Class="Pawn" /><li Class="Verse.Thing" /><li Class="ThingWithComps" />`)
		body.WriteString("</components>")
		saveMods := mods
		if i%2 == 1 {
			saveMods = append(append([]string{}, mods...), "tester.gone")
		}
		e.WriteSave(fmt.Sprintf("Colony%d-%d.rws", colony, i), testkit.SaveSpec{Mods: saveMods, Faction: fmt.Sprintf("Colony %d", colony),
			Seed: fmt.Sprint("seed", colony), PRV: colony, Ticks: int64(60000 * (i + 1)), ModTime: e.Now().Add(-time.Duration(100-i) * time.Minute), Body: body.String()})
	}

	run := func(b *testing.B, fresh bool) {
		for b.Loop() {
			if fresh {
				e.Open()
			}
			if out, code := e.Run("saves", "list"); code > 1 {
				b.Fatalf("saves list: %d\n%s", code, out)
			}
		}
	}
	b.Run("meta-only", func(b *testing.B) { run(b, true) })
	if err := e.App.RunSavesIndex("", nil, nil); err != nil {
		b.Fatal(err)
	}
	// The odd saves' missing Mod owns each colony's 20 classes and 100 Defs (inferred); the
	// even saves have the classes as missing.
	for name, want := range map[string]string{"Colony3-31": "error not-installed tester.gone", "Colony3-30": "error class-missing"} {
		c := checkNamed(b, e, name)
		if got := issueKinds(c); len(got) == 0 || !strings.HasPrefix(got[0], want) {
			b.Fatalf("%s: %q", name, got)
		}
		if o := c.Issues[0].Owns; o == nil || len(o.Classes) != 20 || name == "Colony3-31" && (!o.Inferred || o.Defs[0].Defs != 100) {
			b.Fatalf("%s owns %+v", name, o)
		}
	}
	b.Run("full-read/cold", func(b *testing.B) { run(b, true) })
	b.Run("full-read/warm", func(b *testing.B) { run(b, false) })
}
