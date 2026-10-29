package store_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/harmony"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// TestFactShareScale is opt-in: its synthetic files and database stay under TMP.
func TestFactShareScale(t *testing.T) {
	if os.Getenv("RIMARCHO_FACT_BENCH") == "" {
		t.Skip("set RIMARCHO_FACT_BENCH=1 for the 760,000-row synthetic benchmark")
	}
	e := testkit.New(t)
	e.MustRun("env", "add", "bench", "--game", e.Game, "--user-data", e.UserData)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	s := e.App.Store
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO source(id,environment_id,kind,label,root,status) VALUES('bench-src','','extra','bench',?,'ok')`, e.Root); err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO instance(id,environment_id,lineage_id,source_id,rel_path,status,first_seen,last_seen) VALUES(?,'',?,'bench-src',?,'live',1,1)`)
	if err != nil {
		t.Fatal(err)
	}
	var mods []*analyze.Mod
	for i := 0; i < 4000; i++ {
		id := fmt.Sprintf("synthetic-instance-%08d", i)
		rel := fmt.Sprintf("mod-%04d", i)
		dir := filepath.Join(e.Root, rel)
		if _, err := stmt.Exec(id, id, rel); err != nil {
			t.Fatal(err)
		}
		xml := "<Defs>"
		for j := 0; j < 95; j++ {
			name := ""
			if j%20 == 0 {
				name = fmt.Sprintf(` Name="Parent_%04d_%03d"`, i, j)
			}
			xml += fmt.Sprintf(`<ThingDef%s><defName>Def_%04d_%03d</defName></ThingDef>`, name, i, j)
		}
		e.WriteFile(filepath.Join(dir, "Defs", "Synthetic-content-definitions.xml"), xml+"</Defs>")
		mods = append(mods, &analyze.Mod{Instance: id, Lineage: id, Dir: dir, SnapHash: []byte(id), Pkg: id, Name: id, Pos: i + 1})
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, mm := range []string{"1.6", "1.7"} {
		s.MM = mm
		begin := time.Now()
		out, err := analyze.Run(analyze.Input{Mods: mods, MM: mm, Cached: s.CachedDefs})
		if err != nil {
			t.Fatal(err)
		}
		parsed := time.Since(begin)
		for _, m := range mods {
			a := &analyze.AssemblyFacts{Instance: m.Instance, File: "Assemblies/Synthetic.dll", Assembly: &harmony.Assembly{}}
			for j := 0; j < 20; j++ {
				a.Types = append(a.Types, fmt.Sprintf("Synthetic.Type_%s_%d", m.Instance, j))
			}
			a.IDs = []string{m.Instance}
			out.Assemblies = append(out.Assemblies, a)
		}
		begin = time.Now()
		if _, err := s.SaveAnalysis(st.Ix.Env.ID, "bench", "bench-"+mm, mm, []string{"bench"}, out, e.App.Now(), e.App.Now()); err != nil {
			t.Fatal(err)
		}
		t.Logf("FACTS mm=%s analyze=%s save=%s", mm, parsed, time.Since(begin))
	}
	if _, err := s.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	var pages, size, free int64
	for q, d := range map[string]*int64{"PRAGMA page_count": &pages, "PRAGMA page_size": &size, "PRAGMA freelist_count": &free} {
		if err := s.DB.QueryRow(q).Scan(d); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("SIZE bytes=%d live=%d free=%d", pages*size, (pages-free)*size, free*size)
	for _, q := range []struct {
		name string
		run  func() error
	}{
		{"Name-190", func() error {
			for j := 0; j < 190; j++ {
				if _, err := s.DefsNamed(fmt.Sprintf("Parent_%04d_000", j), true); err != nil {
					return err
				}
			}
			return nil
		}},
		{"defName", func() error { _, err := s.DefsNamed("Def_0001_001", false); return err }},
		{"DefsOf", func() error { _, err := s.DefsOf(mods[0].Instance); return err }},
		{"DefCounts", func() error { _, err := s.DefCounts(); return err }},
		{"TypeOwners", func() error { _, err := s.TypeOwners("Synthetic.Type_" + mods[0].Instance + "_0"); return err }},
	} {
		begin := time.Now()
		runs := 100
		if q.name == "DefCounts" || q.name == "Name-190" {
			runs = 10
		}
		for k := 0; k < runs; k++ {
			if err := q.run(); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("QUERY %s mean=%s", q.name, time.Since(begin)/time.Duration(runs))
	}
	var rows int
	if err := s.DB.QueryRow(`SELECT count(*) FROM def_fact`).Scan(&rows); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	t.Logf("ROWS defs=%d", rows)
}
