package app_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Cache decode, batched Def lookups, inheritance and grouping; selected save only.
// The save is deliberately sparse: Scan27MB measures the dense parser workload.
func BenchmarkSaveContent(b *testing.B) {
	for _, count := range []int{1500, 15000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			e := testkit.New(b)
			mods := []string{"ludeon.rimworld"}
			var body strings.Builder
			for m := range 20 {
				pkg := fmt.Sprintf("tester.content%d", m)
				mods = append(mods, pkg)
				var defs strings.Builder
				defs.WriteString(`<Defs><ThingDef Name="ItemBase` + fmt.Sprint(m) + `" Abstract="True"><category>Item</category></ThingDef>`)
				for i := range count / 20 {
					name := fmt.Sprintf("Content%d_%d", m, i)
					fmt.Fprintf(&defs, `<ThingDef ParentName="ItemBase%d"><defName>%s</defName></ThingDef>`, m, name)
					fmt.Fprintf(&body, `<li><def>%s</def></li>`, name)
				}
				defs.WriteString("</Defs>")
				e.AddWorkshop(fmt.Sprint(3100000000+m), testkit.Mod{PackageID: pkg, Name: fmt.Sprint("Content ", m), Files: map[string]string{"Defs/Many.xml": defs.String()}})
			}
			e.WriteFile(filepath.Join(e.Game, "Data", "Core", "Defs", "Extra.xml"), "<Defs/>")
			e.WriteModsConfig("1.6.4633 rev1261", mods...)
			e.AddEnv(false)
			e.MustRun("scan")
			e.MustRun("analyze")
			e.WriteSave("Many.rws", testkit.SaveSpec{Mods: mods, Body: "<things>" + body.String() + "</things>"})
			s, _, err := e.App.FindSave("Many")
			if err != nil {
				b.Fatal(err)
			}
			if _, err := e.App.SaveFacts(s, true); err != nil {
				b.Fatal(err)
			}
			verify := func(c *app.SaveContentResult, err error) {
				if err != nil || c.Defs != count || len(c.Mods) != 20 || len(c.Totals) != 1 || c.Totals[0].Count != count {
					b.Fatalf("content: %+v %v", c, err)
				}
			}
			verify(e.App.SaveContent(s))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				verify(e.App.SaveContent(s))
			}
		})
	}
}
