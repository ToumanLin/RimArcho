package view_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Synthetic worst case: every one of 1,354 enabled mods has a version warning.
// Keep the harness usable on the parent commit for before/after measurements.
func BenchmarkVersionWarnings1354(b *testing.B) {
	view.SetLang("en")
	e := testkit.New(b)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.5000 rev1")
	packages := []string{"ludeon.rimworld"}
	for i := 0; i < 1354; i++ {
		pkg := fmt.Sprintf("benchmark.mod%d", i)
		e.AddWorkshop(fmt.Sprint(100000+i), testkit.Mod{PackageID: pkg, Name: fmt.Sprintf("Synthetic mod %04d", i), Supported: []string{"1.6"}})
		packages = append(packages, pkg)
	}
	e.WriteModsConfig("1.7.5000 rev1", packages...)
	e.AddEnv(false)
	e.MustRun("scan")
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		b.Fatal(err)
	}
	n := view.BuildNeeds(e.App, ps.State, ps)
	b.Run("BuildNeeds", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			n = view.BuildNeeds(e.App, ps.State, ps)
		}
	})
	payload := struct {
		Problems *view.ListProblems       `json:"problems"`
		Reasons  map[string][]view.Reason `json:"reasons"`
	}{view.ProblemsOf(n), n.Reasons}
	b.Run("BoardProblemsJSON", func(b *testing.B) {
		b.ReportAllocs()
		var out []byte
		for i := 0; i < b.N; i++ {
			out, err = json.Marshal(payload)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(len(out)), "bytes/response")
	})
}
