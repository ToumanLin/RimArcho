package store_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// DefsNamed's SQL before 2026-10-30 (plain JOIN), for comparison: same rows, slower plan.
	plainJoin := func(col, name string) ([]string, error) {
		rows, err := s.DB.Query(`SELECT c.instance_id, f.def_type, f.def_name, f.name, f.parent, f.abstract, f.file, f.line, f.may_require, f.may_require_any
			FROM def_fact f JOIN content_state c ON c.def_set_id=f.set_id WHERE f.`+col+`=? AND f.`+col+`<>'' AND c.game_mm=? ORDER BY c.instance_id, f.rowid`, name, s.MM)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var inst, typ, dn, n, parent, file, mr, mra string
			var abs, line int
			if err := rows.Scan(&inst, &typ, &dn, &n, &parent, &abs, &file, &line, &mr, &mra); err != nil {
				return nil, err
			}
			out = append(out, fmt.Sprintf("%s:%s:%d", inst, file, line))
		}
		return out, rows.Err()
	}
	for j := 0; j < 190; j++ {
		name := fmt.Sprintf("Parent_%04d_000", j)
		ds, err := s.DefsNamed(name, true)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, d := range ds {
			got = append(got, fmt.Sprintf("%s:%s:%d", d.Instance, d.File, d.Line))
		}
		want, err := plainJoin("name", name)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 || fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: DefsNamed %v, plain JOIN %v", name, got, want)
		}
	}
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
		{"Name-190-plainJOIN", func() error {
			for j := 0; j < 190; j++ {
				if _, err := plainJoin("name", fmt.Sprintf("Parent_%04d_000", j)); err != nil {
					return err
				}
			}
			return nil
		}},
		{"defName", func() error { _, err := s.DefsNamed("Def_0001_001", false); return err }},
		{"defName-plainJOIN", func() error { _, err := plainJoin("def_name", "Def_0001_001"); return err }},
		{"DefsOf", func() error { _, err := s.DefsOf(mods[0].Instance); return err }},
		{"DefCounts", func() error { _, err := s.DefCounts(); return err }},
		{"TypeOwners", func() error { _, err := s.TypeOwners("Synthetic.Type_" + mods[0].Instance + "_0"); return err }},
	} {
		begin := time.Now()
		runs := 100
		if q.name == "DefCounts" || strings.HasPrefix(q.name, "Name-190") {
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
