package view_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestOfficialContentFirstOnlyInInactiveColumn(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	for _, pkg := range order.Official[1:] {
		folder := strings.TrimPrefix(pkg, "ludeon.rimworld.")
		e.WriteMod(filepath.Join(e.Game, "Data", folder), testkit.Sample(pkg, folder))
	}
	e.WriteMod(filepath.Join(e.Game, "Data", "Future"), testkit.Sample("ludeon.rimworld.future", "000 Future"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "A"), testkit.Sample("tester.a", "000 A"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Z"), testkit.Sample("tester.z", "ZZZ Z"))
	e.WriteModsConfig("1.6.4633 rev1261")
	e.AddEnv(false)
	e.MustRun("scan")
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	want := append(slices.Clone(order.Official), "ludeon.rimworld.future", "tester.a", "tester.z")
	var inactive []string
	for _, it := range b.Cols[view.Inactive] {
		inactive = append(inactive, it.Package)
	}
	if !slices.Equal(inactive, want) {
		t.Fatalf("inactive %v; want %v", inactive, want)
	}
	if b.St.Ix.Mods[0].PackageID != "tester.a" {
		t.Fatal("the general mod list should still use name order")
	}
	// Enabled rows retain the actual load order, even before auto-sort is run.
	e.WriteModsConfig("1.6.4633 rev1261", "tester.z", "ludeon.rimworld.royalty", "ludeon.rimworld", "tester.a")
	b, err = view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	var active []string
	for _, it := range b.Cols[view.Active] {
		active = append(active, it.Package)
	}
	if want := []string{"tester.z", "ludeon.rimworld.royalty", "ludeon.rimworld", "tester.a"}; !slices.Equal(active, want) {
		t.Fatalf("active %v; want %v", active, want)
	}
}

func TestSortPreviewCanAddCoreWithoutMovingAnything(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.WriteModsConfig("1.6.4633 rev1261")
	e.AddEnv(false)
	e.MustRun("scan")
	p, err := view.DryRun(e.App, view.ListSort)
	if err != nil || !p.Apply {
		t.Fatalf("preview %+v, %v", p, err)
	}
	st, err := e.App.Loadout()
	if err != nil || len(st.List) != 0 {
		t.Fatalf("preview changed workspace: %v", err)
	}
	msg, err := view.Apply(e.App, view.ListSort)
	if err != nil || msg != "sorted: 1 added, 0 moved" {
		t.Fatalf("apply: %q, %v", msg, err)
	}
	st, err = e.App.Loadout()
	if err != nil || len(st.List) != 1 {
		t.Fatalf("Core not added: %v", err)
	}
}
