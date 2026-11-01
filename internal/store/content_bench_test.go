package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkDerivedContent1354: the Mod library content queries before v27 (GROUP BY and EXISTS
// over snapshot_file) and after (snapshot.kinds / langs), and the v27 migration, on 1354
// current snapshots and 4864 in all with 200 file rows each (about the 980,000 rows of a real
// library, 07 §6). Synthetic only:
//
//	go test ./internal/store -run '^$' -bench BenchmarkDerivedContent1354 -benchtime 20x -count 3
func BenchmarkDerivedContent1354(b *testing.B) {
	s, err := Open(filepath.Join(b.TempDir(), "archo.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	for _, q := range []string{
		`WITH RECURSIVE n(i) AS (VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<4864)
		 INSERT INTO snapshot(id, instance_id, scan_id, snapshot_hash) SELECT i, 'inst' || ((i-1) % 1354), 1, x'00' FROM n`,
		`WITH RECURSIVE f(j) AS (VALUES(1) UNION ALL SELECT j+1 FROM f WHERE j<200)
		 INSERT INTO snapshot_file(snapshot_id, rel_path, size, mtime) SELECT s.id, CASE j % 10
		   WHEN 0 THEN '1.6/Defs/ThingDefs_Buildings/XYZ_Buildings_' || j || '.xml'
		   WHEN 1 THEN '1.6/Languages/ChineseSimplified (简体中文)/DefInjected/ThingDef/XYZ_' || j || '.xml'
		   WHEN 2 THEN 'Patches/XYZ_Patch_' || j || '.xml'
		   WHEN 3 THEN 'Sounds/XYZ/Ambient_' || j || '.ogg'
		   WHEN 4 THEN CASE WHEN s.id % 3 = 0 THEN '1.6/Assemblies/XYZ_' || j || '.dll' ELSE 'About/Notes_' || j || '.txt' END
		   ELSE 'Textures/Things/Building/Furniture/XYZ_Item' || j || '_north.png' END, 1, 1
		 FROM snapshot s CROSS JOIN f`,
		`WITH RECURSIVE n(i) AS (VALUES(0) UNION ALL SELECT i+1 FROM n WHERE i<1353)
		 INSERT INTO instance(id, environment_id, lineage_id, source_id, rel_path, real_path, status, current_snapshot_id, first_seen, last_seen)
		 SELECT 'inst' || i, '', 'l' || i, 's', 'm' || i, 'm' || i, 'live', 3511 + i, 1, 1 FROM n`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			b.Fatal(err)
		}
	}
	tx, err := s.DB.Begin()
	if err != nil {
		b.Fatal(err)
	}
	if err := migrateV27(tx); err != nil {
		b.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	snaps := map[string]int64{}
	var ids []any
	for i := 0; i < 1354; i++ {
		snaps[fmt.Sprint("inst", i)] = int64(3511 + i)
		ids = append(ids, int64(3511+i))
	}
	var rels [][]string // the file lists of a few snapshots, as a scan has them
	for _, id := range ids[:10] {
		rows, err := s.DB.Query(`SELECT rel_path FROM snapshot_file WHERE snapshot_id=?`, id)
		if err != nil {
			b.Fatal(err)
		}
		var rs []string
		for rows.Next() {
			var r string
			rows.Scan(&r)
			rs = append(rs, r)
		}
		rows.Close()
		rels = append(rels, rs)
	}
	oldKinds := func(b *testing.B) {
		for rest := ids; len(rest) > 0; {
			n := min(len(rest), 500)
			rows, err := s.DB.Query(`SELECT snapshot_id, MAX(rel_path LIKE '%.dll'),
			  MAX((rel_path LIKE 'defs/%' OR rel_path LIKE '%/defs/%' OR rel_path LIKE 'patches/%' OR rel_path LIKE '%/patches/%') AND rel_path LIKE '%.xml'),
			  MAX(rel_path LIKE 'languages/%' OR rel_path LIKE '%/languages/%')
			FROM snapshot_file WHERE snapshot_id IN (`+placeholders(n)+`) GROUP BY snapshot_id`, rest[:n]...)
			if err != nil {
				b.Fatal(err)
			}
			for rows.Next() {
			}
			rows.Close()
			rest = rest[n:]
		}
	}
	oldPNG := func(b *testing.B) {
		rows, err := s.DB.Query(`SELECT i.current_snapshot_id FROM instance i WHERE i.current_snapshot_id IS NOT NULL
			AND EXISTS(SELECT 1 FROM snapshot_file f WHERE f.snapshot_id=i.current_snapshot_id AND f.rel_path LIKE '%textures/%.png')`)
		if err != nil {
			b.Fatal(err)
		}
		for rows.Next() {
		}
		rows.Close()
	}
	oldLangs := func(b *testing.B) {
		for _, id := range ids[:100] {
			rows, err := s.DB.Query(`SELECT rel_path FROM snapshot_file WHERE snapshot_id=? AND lower(rel_path) LIKE '%languages/%'`, id)
			if err != nil {
				b.Fatal(err)
			}
			for rows.Next() {
				var rel string
				rows.Scan(&rel)
				_ = strings.Split(rel, "/")
			}
			rows.Close()
		}
	}
	for _, c := range []struct {
		name string
		f    func(*testing.B)
	}{
		{"ContentKinds/old", oldKinds},
		{"ContentKinds/v27", func(b *testing.B) {
			if _, err := s.ContentKinds(snaps); err != nil {
				b.Fatal(err)
			}
		}},
		{"SnapshotsWithPNG/old", oldPNG},
		{"SnapshotsWithPNG/v27", func(b *testing.B) {
			if _, err := s.SnapshotsWithPNG(); err != nil {
				b.Fatal(err)
			}
		}},
		{"Languages100/old", oldLangs},
		{"Languages100/v27", func(b *testing.B) {
			for _, id := range ids[:100] {
				if _, err := s.Languages(id.(int64)); err != nil {
					b.Fatal(err)
				}
			}
		}},
		{"deriveContent1354", func(b *testing.B) { // a full scan's extra work: every snapshot new
			for i := range 1354 {
				deriveContent(rels[i%len(rels)])
			}
		}},
		{"migrateV27", func(b *testing.B) {
			tx, err := s.DB.Begin()
			if err != nil {
				b.Fatal(err)
			}
			defer tx.Rollback()
			if err := migrateV27(tx); err != nil {
				b.Fatal(err)
			}
		}},
	} {
		b.Run(c.name, func(b *testing.B) {
			for range b.N {
				c.f(b)
			}
		})
	}
}
