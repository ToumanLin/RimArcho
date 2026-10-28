package view_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// A list entry with nothing installed shows a name when one is known, and a mod removed
// from disk keeps its name and says it is gone.
func TestMissingRowsKeepTheModName(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	dir := e.AddWorkshop("3000000001", testkit.Mod{PackageID: "tester.gone", Name: "Gone Mod"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.gone", "tester.never")
	e.AddEnv(false)
	e.MustRun("scan")
	e.Remove(dir)
	e.MustRun("scan")

	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, it := range b.Cols[view.Active] {
		switch {
		case it.Missing:
			got[it.Ref] = it.Name
		case it.Gone:
			got[it.Package] = it.Name
		}
	}
	if got["tester.gone"] != "Gone Mod" || got["pkg:tester.never"] != "tester.never" {
		t.Fatalf("rows not installed: %v", got)
	}
}
