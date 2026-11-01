package store_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// TestIntKeyConvertFile converts an existing archo.db in place to the layout decided in
// docs/research/2026-10-30_archo-db-整数键.md §10 (one zstd list per snapshot without dedup,
// plus a file list per fact set) and logs sizes and times. Point RIMARCHO_INTKEY_DB at a
// copy: the file is rewritten. For the VM's real library the copy stays inside the VM.
func TestIntKeyConvertFile(t *testing.T) {
	path := os.Getenv("RIMARCHO_INTKEY_DB")
	if path == "" {
		t.Skip("set RIMARCHO_INTKEY_DB to a copy of archo.db")
	}
	begin := time.Now()
	s, err := store.Open(path) // brings it to the current schema first
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("OPEN %s schema=%s", time.Since(begin).Round(time.Millisecond), s.Meta("schema_version"))
	// The Mod library content lookups on the real rows: the queries before v27 and the columns.
	snaps := map[string]int64{}
	rows, err := s.DB.Query(`SELECT id, current_snapshot_id FROM instance WHERE current_snapshot_id IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		var snap int64
		rows.Scan(&id, &snap)
		snaps[id] = snap
		ids = append(ids, "?")
	}
	rows.Close()
	var args []any
	for _, v := range snaps {
		args = append(args, v)
	}
	timeIt := func(name string, f func() error) {
		var best time.Duration
		for i := 0; i < 5; i++ {
			b := time.Now()
			if err := f(); err != nil {
				t.Fatal(err)
			}
			if d := time.Since(b); i == 0 || d < best {
				best = d
			}
		}
		t.Logf("QUERY %s best of 5 %s", name, best.Round(10*time.Microsecond))
	}
	oldKinds := func() error { // batches of 500, as ContentKinds did
		for rest := args; len(rest) > 0; {
			n := min(len(rest), 500)
			r, err := s.DB.Query(`SELECT snapshot_id, MAX(rel_path LIKE '%.dll'),
			  MAX((rel_path LIKE 'defs/%' OR rel_path LIKE '%/defs/%' OR rel_path LIKE 'patches/%' OR rel_path LIKE '%/patches/%') AND rel_path LIKE '%.xml'),
			  MAX(rel_path LIKE 'languages/%' OR rel_path LIKE '%/languages/%')
			FROM snapshot_file WHERE snapshot_id IN (`+strings.Join(ids[:n], ",")+`) GROUP BY snapshot_id`, rest[:n]...)
			if err != nil {
				return err
			}
			for r.Next() {
			}
			r.Close()
			rest = rest[n:]
		}
		return nil
	}
	oldPNG := func() error {
		r, err := s.DB.Query(`SELECT i.current_snapshot_id FROM instance i WHERE i.current_snapshot_id IS NOT NULL
			AND EXISTS(SELECT 1 FROM snapshot_file f WHERE f.snapshot_id=i.current_snapshot_id AND f.rel_path LIKE '%textures/%.png')`)
		if err != nil {
			return err
		}
		for r.Next() {
		}
		return r.Close()
	}
	t.Logf("SCOPE %d current snapshots", len(snaps))
	timeIt("ContentKinds/old", oldKinds)
	timeIt("ContentKinds/v27", func() error { _, err := s.ContentKinds(snaps); return err })
	timeIt("SnapshotsWithPNG/old", oldPNG)
	timeIt("SnapshotsWithPNG/v27", func() error { _, err := s.SnapshotsWithPNG(); return err })
	s.Close()

	db := ikOpen(t, path)
	defer db.Close()
	var counts []string
	for _, q := range []string{"snapshot", "snapshot_file", "fact_set", "content_state", "def_fact", "type_fact", "harmony_fact", "assembly_fact", "patch_fact"} {
		var n int64
		db.QueryRow(`SELECT count(*) FROM ` + q).Scan(&n)
		counts = append(counts, fmt.Sprintf("%s=%d", q, n))
	}
	var nonsem, nsBytes, relBytes, fileBytes int64
	db.QueryRow(`SELECT count(*), coalesce(sum(length(CAST(rel_path AS BLOB))),0) FROM snapshot_file WHERE hash IS NULL AND lower(rel_path) NOT LIKE '%.dll'`).Scan(&nonsem, &nsBytes)
	db.QueryRow(`SELECT coalesce(sum(length(CAST(rel_path AS BLOB))),0) FROM snapshot_file`).Scan(&relBytes)
	db.QueryRow(`SELECT coalesce(sum(length(CAST(file AS BLOB))),0) FROM def_fact`).Scan(&fileBytes)
	t.Logf("ROWS %s", strings.Join(counts, " "))
	t.Logf("SHAPE rel_path=%.1fMB blobbed rows=%d (rel %.1fMB) def_fact.file=%.1fMB", float64(relBytes)/1e6, nonsem, float64(nsBytes)/1e6, float64(fileBytes)/1e6)
	vb := time.Now()
	if _, err := db.Exec(`VACUUM`); err != nil {
		t.Fatal(err)
	}
	db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	t.Logf("VACUUM before %s", time.Since(vb).Round(time.Millisecond))
	ikLogSizes(t, db, "v27", ikSizes(t, db))

	l := ikLayout{name: "decided", blob: 1, kinds: true, path: 4}
	conv, bst := ikConvert(t, db, l)
	db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	pre := ikSizes(t, db)
	vb = time.Now()
	if _, err := db.Exec(`VACUUM`); err != nil {
		t.Fatal(err)
	}
	db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	t.Logf("CONVERT %s convert=%s file_before_vacuum=%.1fMB live_before_vacuum=%.1fMB vacuum=%s", l.name, conv.Round(time.Millisecond), float64(pre.file)/1e6, float64(pre.live)/1e6, time.Since(vb).Round(time.Millisecond))
	t.Logf("BLOB lists=%d raw=%.1fMB zstd=%.1fMB stored=%.1fMB", bst.lists, float64(bst.raw)/1e6, float64(bst.zstd)/1e6, float64(bst.stored)/1e6)
	ikLogSizes(t, db, l.name, ikSizes(t, db))
	// every row that went into a list comes back out of it
	var back int64
	r, err := db.Query(`SELECT data FROM snapshot_list`)
	if err != nil {
		t.Fatal(err)
	}
	for r.Next() {
		var data []byte
		r.Scan(&data)
		if err := ikDecode(data, func(string, int64) { back++ }); err != nil {
			t.Fatal(err)
		}
	}
	r.Close()
	if back != nonsem {
		t.Fatalf("lists hold %d rows, %d went in", back, nonsem)
	}
	t.Logf("CHECK lists decode to %d rows", back)
}
