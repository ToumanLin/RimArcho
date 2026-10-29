package store

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// v24DB builds a schema v24 database. noNamed drops def_fact_mm_named, as on databases that
// took the texture branch's v15 (2026-10-22) and never ran the logs branch's v15.
func v24DB(t *testing.T, path string, noNamed bool, rows ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	steps := []string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10, schemaV11, schemaV12,
		schemaV13, schemaV14, schemaV15, schemaV16, schemaV17, schemaV18, schemaV19, schemaV20, schemaV21, schemaV22, schemaV23, schemaV24}
	if _, err := db.Exec(strings.Join(steps, "")); err != nil {
		t.Fatal(err)
	}
	if noNamed {
		if _, err := db.Exec(`DROP INDEX def_fact_mm_named`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', '24')`); err != nil {
		t.Fatal(err)
	}
	for _, q := range rows {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func indexes(t *testing.T, s *Store) map[string]bool {
	t.Helper()
	rows, err := s.DB.Query(`SELECT name FROM sqlite_master WHERE type='index'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		rows.Scan(&n)
		out[n] = true
	}
	return out
}

// 02 §7, 03 §3 (v25): the unused fact indexes go, def_fact_mm_named comes back where a
// version clash left it out, and stored log reports are compressed and still readable.
func TestMigrateV25(t *testing.T) {
	for _, noNamed := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "archo.db")
		report := `{"Path":"C:\\Logs\\Player.log","Clusters":[` + strings.Repeat(`{"First":"Exception in Verse.Foo"},`, 2000) + `{}]}`
		v24DB(t, path, noNamed, fmt.Sprintf(`INSERT INTO log_cache(path, size, mtime, version, input_key, report) VALUES('C:\Logs\Player.log', 10, 20, '1.6', 'k', CAST('%s' AS BLOB))`, report))
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		ix := indexes(t, s)
		for _, gone := range []string{"def_fact_name", "type_fact_name", "type_fact_instance", "harmony_fact_instance", "harmony_fact_target"} {
			if ix[gone] {
				t.Errorf("noNamed=%v: index %s kept", noNamed, gone)
			}
		}
		for _, kept := range []string{"def_fact_mm_named", "def_fact_mm", "def_fact_mm_name", "def_fact_instance", "type_fact_mm", "type_fact_inst_mm", "harmony_fact_mm", "harmony_fact_inst_mm", "harmony_id_id"} {
			if !ix[kept] {
				t.Errorf("noNamed=%v: index %s missing", noNamed, kept)
			}
		}
		var raw []byte
		s.DB.QueryRow(`SELECT report FROM log_cache`).Scan(&raw)
		if !isGzip(raw) || len(raw) >= len(report)/5 {
			t.Errorf("noNamed=%v: stored report not compressed: %d of %d bytes", noNamed, len(raw), len(report))
		}
		c, err := s.LogCache(`C:\Logs\Player.log`)
		if err != nil || c == nil || string(c.Report) != report || c.Size != 10 || c.Version != "1.6" {
			t.Errorf("noNamed=%v: cached report after migration: %v %+v", noNamed, err, c)
		}
		s.Close()
	}
}

// 03 §6.5: reports are stored gzipped and read back unchanged; a report that cannot be
// decompressed counts as no cache, so the log is parsed again.
func TestLogCacheCompressed(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rep := []byte(`{"Clusters":[` + strings.Repeat(`{"First":"Could not resolve cross-reference"},`, 500) + `{}]}`)
	if err := s.PutLogCache(&LogCache{Path: "p", Size: 1, MTime: 2, Version: "1.7", InputKey: "k", Report: rep}); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	s.DB.QueryRow(`SELECT report FROM log_cache WHERE path='p'`).Scan(&raw)
	if !isGzip(raw) || len(raw) > len(rep)/5 {
		t.Fatalf("stored %d bytes for a %d-byte report", len(raw), len(rep))
	}
	c, err := s.LogCache("p")
	if err != nil || c == nil || !bytes.Equal(c.Report, rep) {
		t.Fatalf("read back: %v %v", err, c)
	}
	s.DB.Exec(`UPDATE log_cache SET report=X'1f8b0000' WHERE path='p'`)
	if c, err := s.LogCache("p"); err != nil || c != nil {
		t.Fatalf("broken report: %v %v, want no cache", err, c)
	}
}

func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

func bigWrite(t *testing.T, s *Store, n int) {
	t.Helper()
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	ins, _ := tx.Prepare(`INSERT INTO def_fact(instance_id, def_type, def_name, file, line, game_mm) VALUES(?,?,?,?,?,?)`)
	for i := 0; i < n; i++ {
		if _, err := ins.Exec(fmt.Sprintf("inst%05d", i/100), "ThingDef", fmt.Sprintf("Def_%d", i), "1.6/Defs/ThingDefs/Things.xml", i, "1.7"); err != nil {
			t.Fatal(err)
		}
	}
	ins.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// 02 §7: after a large write the WAL file is truncated instead of keeping its size while
// connections stay open; with another connection reading, TruncateWAL does not wait and
// journal_size_limit cuts the file back on the next reset.
func TestTruncateWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bigWrite(t, s, 150000)
	if n := fileSize(path + "-wal"); n < 20<<20 {
		t.Fatalf("WAL after the write is %d bytes; the test needs it above the 16 MB limit", n)
	}
	s.TruncateWAL()
	if n := fileSize(path + "-wal"); n != 0 {
		t.Fatalf("WAL after TruncateWAL is %d bytes, want 0", n)
	}

	// A reader in another connection holds an older state.
	r, err := OpenSession(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	bigWrite(t, s, 150000)
	tx, err := r.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	var n int
	tx.QueryRow(`SELECT count(*) FROM def_fact`).Scan(&n)
	bigWrite(t, s, 1000)
	start := time.Now()
	s.TruncateWAL()
	if d := time.Since(start); d > time.Second {
		t.Fatalf("TruncateWAL waited %v for the reader", d)
	}
	tx.Rollback()
	s.TruncateWAL()
	bigWrite(t, s, 10)
	if n := fileSize(path + "-wal"); n > 16<<20 {
		t.Fatalf("WAL after the reader finished is %d bytes, want at most the 16 MB limit", n)
	}
}

// 03 §3: every fact query still finds an index after v25 dropped the single-column ones.
func TestFactQueriesUseIndexes(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, q := range []string{
		`SELECT * FROM def_fact WHERE instance_id='a' AND game_mm='1.7' ORDER BY rowid`,
		`SELECT * FROM def_fact WHERE def_name='a' AND game_mm='1.7' ORDER BY rowid`,
		`SELECT * FROM def_fact WHERE name='a' AND name<>'' AND game_mm='1.7' ORDER BY rowid`,
		`SELECT instance_id, count(*) FROM def_fact WHERE game_mm='1.7' GROUP BY instance_id`,
		`DELETE FROM def_fact WHERE instance_id='a'`,
		`SELECT DISTINCT instance_id, file FROM type_fact WHERE type_name IN ('a', 'b') AND game_mm='1.7'`,
		`DELETE FROM type_fact WHERE instance_id='a'`,
		`SELECT * FROM harmony_fact WHERE game_mm='1.7' AND target='a'`,
		`SELECT * FROM harmony_fact WHERE game_mm='1.7' AND instance_id='a'`,
		`DELETE FROM harmony_fact WHERE instance_id='a'`,
		`SELECT DISTINCT instance_id FROM harmony_id WHERE id='a' AND game_mm='1.7'`,
	} {
		rows, err := s.DB.Query(`EXPLAIN QUERY PLAN ` + q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			rows.Scan(&id, &parent, &unused, &detail)
			plan = append(plan, detail)
		}
		rows.Close()
		p := strings.Join(plan, "; ")
		if !strings.Contains(p, "INDEX") {
			t.Errorf("%s: no index (%s)", q, p)
		}
	}
}
