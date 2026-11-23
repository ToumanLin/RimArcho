package view

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// A Mod Steam no longer has (delisted, deleted or private) is not offered a download, whether
// the list has it or another Mod requires it; a local Mod with no Workshop ID is not either.
// Each list entry that is not installed carries its remove reference (16 §6.1).
func TestReadSortFixesDelisted(t *testing.T) {
	SetLang("en")
	n := app.SortNotices{
		MissingMods: []app.MissingMod{
			{SortMod: app.SortMod{Name: "Delisted", PackageID: "tester.delisted", WorkshopID: "9001"}},
			{SortMod: app.SortMod{Name: "Still There", PackageID: "tester.still", WorkshopID: "9002"}},
			{SortMod: app.SortMod{Name: "Local Only", PackageID: "tester.local"}},
		},
		MissingDependencies: []app.MissingDependency{
			{Kind: "workshop", Name: "Gone Dep", PackageID: "tester.gonedep", WorkshopID: "9003", RequiredBy: []string{"User"}},
		},
	}
	gone := map[string]bool{"9001": true, "9003": true}
	f := readSortFixes(n, gone, func(pkg string) string { return "pkg:" + pkg })
	if strings.Join(f.wsIDs, ",") != "9002" || len(f.get) != 1 || f.get[0].Remove != "pkg:tester.still" {
		t.Fatalf("download: %v %+v", f.wsIDs, f.get)
	}
	why := map[string]SortStepItem{}
	for _, it := range f.unfixable {
		why[it.Name] = it
	}
	for _, name := range []string{"Delisted", "Gone Dep"} {
		if !strings.Contains(why[name].Why, "removed from the Workshop") {
			t.Fatalf("%s: %+v", name, f.unfixable)
		}
	}
	if why["Delisted"].Remove != "pkg:tester.delisted" || why["Local Only"].Remove != "pkg:tester.local" || why["Gone Dep"].Remove != "" {
		t.Fatalf("remove references: %+v", f.unfixable)
	}
	if why["Local Only"].Why != "in the list, no known source" {
		t.Fatalf("local-only: %+v", why["Local Only"])
	}
}
