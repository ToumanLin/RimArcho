package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	id, _, err := ensureFactSet(tx, "wal", "def", fmt.Sprint(time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	ins, err := tx.Prepare(`INSERT INTO def_fact(set_id,def_type,def_name,file_ix,line) VALUES(?,?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err := ins.Exec(id, "ThingDef", fmt.Sprintf("Def_%d", i), 0, i); err != nil {
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
	bigWrite(t, s, 330000)
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
	bigWrite(t, s, 330000)
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

// 03 §3: Def lookups by name start from the name index and then find each set's version
// view. With a plain JOIN, the ORDER BY made SQLite walk the version's whole content_state
// and probe the name index once per row (07 §6.2).
func TestDefsNamedPlanStartsFromName(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, c := range []struct{ q, outer string }{
		{defsByDefNameQ, "SEARCH f USING INDEX def_fact_lookup (def_name=?)"},
		{defsByNameQ, "SEARCH f USING INDEX def_fact_named (name=?)"},
	} {
		rows, err := s.DB.Query(`EXPLAIN QUERY PLAN `+c.q, "a", "1.7")
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			rows.Scan(&id, &parent, &unused, &detail)
			plan = append(plan, detail)
		}
		rows.Close()
		if len(plan) < 2 || plan[0] != c.outer || !strings.HasPrefix(plan[1], "SEARCH c USING INDEX content_state_def (def_set_id=? AND game_mm=?)") {
			t.Errorf("%s:\n%s", c.q, strings.Join(plan, "\n"))
		}
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
		`SELECT ` + defCols + defView + `WHERE c.instance_id='a' AND c.game_mm='1.7' ORDER BY f.rowid`,
		`SELECT ` + defCols + defView + `WHERE f.def_name='a' AND c.game_mm='1.7' ORDER BY f.rowid`,
		`SELECT ` + defCols + defView + `WHERE f.name='a' AND f.name<>'' AND c.game_mm='1.7' ORDER BY f.rowid`,
		`SELECT c.instance_id,count(*)` + defView + `WHERE c.game_mm='1.7' GROUP BY c.instance_id`,
		`DELETE FROM fact_set WHERE instance_id='a'`,
		`DELETE FROM def_fact WHERE set_id=1`,
		`SELECT DISTINCT c.instance_id,f.set_id,f.file_ix FROM type_fact f CROSS JOIN content_state c ON c.dll_set_id=f.set_id WHERE f.type_name IN ('a','b') AND c.game_mm='1.7'`,
		`DELETE FROM type_fact WHERE set_id=1`,
		`SELECT * FROM harmony_fact f JOIN content_state c ON c.dll_set_id=f.set_id WHERE c.game_mm='1.7' AND f.target='a'`,
		`SELECT * FROM harmony_fact f JOIN content_state c ON c.dll_set_id=f.set_id WHERE c.game_mm='1.7' AND c.instance_id='a'`,
		`DELETE FROM harmony_fact WHERE set_id=1`,
		`SELECT DISTINCT c.instance_id FROM harmony_id f CROSS JOIN content_state c ON c.dll_set_id=f.set_id WHERE f.id='a' AND c.game_mm='1.7'`,
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
