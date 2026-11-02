package view_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func BenchmarkSharedFilters1354(b *testing.B) {
	e := testkit.New(b)
	packages := []string{"ludeon.rimworld"}
	for i := 0; i < 1354; i++ {
		pkg, name := fmt.Sprintf("tester.mod%d", i), fmt.Sprintf("Mod %04d", i)
		e.WriteMod(filepath.Join(e.Game, "Mods", name), testkit.Sample(pkg, name))
		packages = append(packages, pkg)
	}
	e.WriteModsConfig("1.6.4633 rev1261", packages...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("plan", "new", "Other colony", "--from", "current")
	e.MustRun("plan", "new", "Empty list")
	board, err := view.BuildBoard(e.App)
	if err != nil {
		b.Fatal(err)
	}
	lib, err := e.App.Library()
	if err != nil {
		b.Fatal(err)
	}
	b.Run("BoardBuild", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := view.BuildBoard(e.App); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("BoardFilter", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			view.FilterBoard(board, "", nil, view.MatchFacts{}, true, "1.6")
		}
	})
	b.Run("LibraryFilter", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := view.BuildLibrary(lib, "", nil, ""); err != nil {
				b.Fatal(err)
			}
		}
	})
}
