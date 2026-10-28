package view_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// A legacy database can still show 67 questions until its next scan. Ordinary
// page construction must stay cheap even in that case; evidence is loaded on click.
func BenchmarkIdentityQuestionPage1354(b *testing.B) {
	e := testkit.New(b)
	for n := 0; n < 1354; n++ {
		name := fmt.Sprintf("Mod %04d", n)
		e.WriteMod(filepath.Join(e.Game, "Mods", name), testkit.Mod{PackageID: fmt.Sprintf("tester.mod%d", n), Name: name})
	}
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		b.Fatal(err)
	}
	var candidate string
	for _, m := range st.Ix.Mods {
		if m.Name == "Mod 0000" {
			candidate = m.ID
		}
	}
	count := 0
	for _, m := range st.Ix.Mods {
		if m.ID == candidate || count == 67 {
			continue
		}
		if _, err := e.App.Store.DB.Exec(`INSERT INTO identity_question(environment_id,scan_id,instance_id,observation_snapshot_id,candidate_instance_id,score) VALUES(?,1,?,?,?,1)`, st.Ix.Env.ID, m.ID, m.SnapID, candidate); err != nil {
			b.Fatal(err)
		}
		count++
	}
	ps := &app.ProblemSet{}
	if n := view.BuildNeeds(e.App, st, ps); n.Count != 67 {
		b.Fatalf("questions=%d", n.Count)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		view.BuildNeeds(e.App, st, ps)
	}
}
