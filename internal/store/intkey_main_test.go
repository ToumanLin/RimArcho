package store_test

// TestIntKeyResearch: see intkey_research_test.go. Run (PowerShell), with TMP/TEMP on the
// scratch folder:
//
//	$env:RIMARCHO_INTKEY_BENCH = '1'
//	go test ./internal/store -run '^TestIntKeyResearch$' -v -count=1 -timeout 60m

import (
	"database/sql"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
)

func ikOpen(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(0)&_pragma=journal_mode(WAL)&_pragma=journal_size_limit(16777216)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func ikDisk(t *testing.T, limit int64) int64 {
	t.Helper()
	root := os.Getenv("RIMARCHO_INTKEY_DISK_ROOT")
	if root == "" {
		root = os.TempDir()
	}
	var n int64
	filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if i, err := d.Info(); err == nil {
				n += i.Size()
			}
		}
		return nil
	})
	if n > limit {
		t.Fatalf("disk cap: %s holds %d MB > %d MB", root, n>>20, limit>>20)
	}
	return n
}

func ikMedian(xs []time.Duration) time.Duration {
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[len(s)/2]
}

func TestIntKeyResearch(t *testing.T) {
	if os.Getenv("RIMARCHO_INTKEY_BENCH") == "" {
		t.Skip("set RIMARCHO_INTKEY_BENCH=1 for the integer-key research benchmark")
	}
	scale := 1.0
	if v := os.Getenv("RIMARCHO_INTKEY_SCALE"); v != "" {
		scale, _ = strconv.ParseFloat(v, 64)
	}
	limit := int64(1400) << 20
	if v := os.Getenv("RIMARCHO_INTKEY_DISK_MB"); v != "" {
		mb, _ := strconv.ParseInt(v, 10, 64)
		limit = mb << 20
	}
	rounds := 3
	// The library is written in the v27 layout the prototypes convert (schema v28 already
	// is the chosen one; TestV28Formal measures it).
	root := t.TempDir()
	const envID = "bench-env"
	s := &store.Store{DB: store.NewV27DB(t, filepath.Join(root, "base.db"))}
	defer s.DB.Close()
	if _, err := s.DB.Exec(`INSERT INTO environment(id,name,game_root,game_version,created_at) VALUES(?,?,?,?,1)`, envID, "bench", "D:/bench", "1.6.4633"); err != nil {
		t.Fatal(err)
	}
	p := ikPlanLibrary(scale, envID)
	begin := time.Now()
	smp := ikLoad(t, s.DB, p, envID)
	t.Logf("BUILD %s", time.Since(begin).Round(time.Millisecond))

	// samples for CachedDefs and the write tests, read from the baseline
	r := rand.New(rand.NewPCG(9, 9))
	{
		ls, err := ikLines(s.DB.Query(`SELECT c.instance_id, c.content_key FROM content_state c WHERE c.game_mm='1.6' AND c.instance_id NOT IN (SELECT id FROM bench_active) ORDER BY c.instance_id`))
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range ls {
			a, b, _ := strings.Cut(l, "\x1f")
			smp.cached = append(smp.cached, [2]string{a, b})
		}
		for k := 0; k < 30; k++ {
			snap := smp.snaps[r.IntN(len(smp.snaps))]
			var inst string
			s.DB.QueryRow(`SELECT instance_id FROM snapshot WHERE id=?`, snap).Scan(&inst)
			rows, err := s.DB.Query(`SELECT rel_path, size, mtime, hash FROM snapshot_file WHERE snapshot_id=? ORDER BY rel_path`, snap)
			if err != nil {
				t.Fatal(err)
			}
			m := ikManifest{inst: inst}
			for rows.Next() {
				var x ikRow
				rows.Scan(&x.rel, &x.size, &x.mtime, &x.hash)
				m.rows = append(m.rows, x)
			}
			rows.Close()
			smp.manifests = append(smp.manifests, m)
		}
		var sets []int64
		rows, _ := s.DB.Query(`SELECT id FROM fact_set WHERE kind='def' ORDER BY id`)
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			sets = append(sets, id)
		}
		rows.Close()
		for k := 0; k < 30; k++ {
			set := sets[r.IntN(len(sets))]
			d := ikDefSet{}
			s.DB.QueryRow(`SELECT instance_id FROM fact_set WHERE id=?`, set).Scan(&d.inst)
			rows, _ := s.DB.Query(`SELECT def_type, def_name, name, parent, abstract, file, line, may_require, may_require_any FROM def_fact WHERE set_id=? ORDER BY rowid`, set)
			for rows.Next() {
				var x ikDef
				rows.Scan(&x.typ, &x.defName, &x.name, &x.parent, &x.abstract, &x.file, &x.line, &x.mr, &x.mra)
				d.defs = append(d.defs, x)
			}
			rows.Close()
			smp.defSets = append(smp.defSets, d)
		}
	}
	for _, q := range []string{`PRAGMA wal_checkpoint(TRUNCATE)`, `VACUUM`, `PRAGMA wal_checkpoint(TRUNCATE)`} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	{
		var parts []string
		for _, tb := range []string{"instance", "snapshot", "snapshot_file", "fact_set", "content_state", "def_fact", "type_fact", "harmony_fact", "harmony_id", "assembly_fact", "patch_fact", "finding"} {
			var n int
			s.DB.QueryRow(`SELECT count(*) FROM ` + tb).Scan(&n)
			parts = append(parts, tb+"="+strconv.Itoa(n))
		}
		var nonsem, fileBytes, relBytes, nsBytes int64
		s.DB.QueryRow(`SELECT count(*), coalesce(sum(length(CAST(rel_path AS BLOB))),0) FROM snapshot_file WHERE hash IS NULL`).Scan(&nonsem, &nsBytes)
		s.DB.QueryRow(`SELECT coalesce(sum(length(CAST(rel_path AS BLOB))),0) FROM snapshot_file`).Scan(&relBytes)
		s.DB.QueryRow(`SELECT coalesce(sum(length(CAST(file AS BLOB))),0) FROM def_fact`).Scan(&fileBytes)
		var defs, instBytes, distinctFiles, distinctRel, active int64
		s.DB.QueryRow(`SELECT count(*) FROM def_fact`).Scan(&defs)
		s.DB.QueryRow(`SELECT count(DISTINCT file) FROM def_fact`).Scan(&distinctFiles)
		s.DB.QueryRow(`SELECT count(DISTINCT rel_path) FROM snapshot_file`).Scan(&distinctRel)
		s.DB.QueryRow(`SELECT count(*) FROM def_fact f JOIN fact_set s ON s.id=f.set_id WHERE s.instance_id IN (SELECT id FROM bench_active)`).Scan(&active)
		s.DB.QueryRow(`SELECT (SELECT coalesce(sum(length(instance_id)),0) FROM fact_set)+(SELECT coalesce(sum(length(instance_id)),0) FROM content_state)+(SELECT coalesce(sum(length(instance_id)),0) FROM patch_fact)+(SELECT coalesce(sum(length(instance_id)),0) FROM snapshot)+(SELECT coalesce(sum(length(subject_instance)),0) FROM finding)+(SELECT coalesce(sum(length(instance_id)),0) FROM change_event)`).Scan(&instBytes)
		var rows int64
		s.DB.QueryRow(`SELECT count(*) FROM snapshot_file`).Scan(&rows)
		t.Logf("ROWS %s", strings.Join(parts, " "))
		t.Logf("SHAPE snapshot_file rel_path=%.1fMB (avg %.1f B) non-semantic=%d (%.1f%%, rel %.1fMB) distinct_rel=%d | def_fact file=%.1fMB (avg %.1f B) distinct_file=%d active_defs=%d (%.1f%%) | instance-id text in fact_set+content_state+patch_fact+snapshot+finding+change_event=%.2fMB",
			float64(relBytes)/1e6, float64(relBytes)/float64(max(1, rows)), nonsem, 100*float64(nonsem)/float64(max(1, rows)), float64(nsBytes)/1e6, distinctRel,
			float64(fileBytes)/1e6, float64(fileBytes)/float64(max(1, defs)), distinctFiles, active, 100*float64(active)/float64(max(1, defs)), float64(instBytes)/1e6)
	}
	t.Logf("DISK after build: %d MB", ikDisk(t, limit)>>20)

	layouts := []ikLayout{
		{name: "v26"},
		{name: "v26-copy"}, // same layout, opened like the variants: the noise floor
		{name: "ik", ik: true},
		{name: "path-global", path: 1},
		{name: "path-hash", path: 2},
		{name: "ik+path-mod", ik: true, path: 3},
		{name: "blob", blob: 1, kinds: true},
		{name: "blob-dedup", blob: 2, kinds: true},
		{name: "slim-names", slim: 1},
		{name: "slim-parent", slim: 2},
		{name: "setfiles", path: 4},
		{name: "combo-global", blob: 2, kinds: true, path: 1, slim: 2},
		{name: "combo-setfiles", blob: 2, kinds: true, path: 4, slim: 2},
		{name: "combo-hash", blob: 2, kinds: true, path: 2, slim: 2},
		{name: "combo-ik-mod", ik: true, blob: 2, kinds: true, path: 3, slim: 2},
		{name: "combo-names", blob: 2, kinds: true, path: 2, slim: 1},
	}
	if v := os.Getenv("RIMARCHO_INTKEY_LAYOUTS"); v != "" {
		want := strings.Split(v, ",")
		layouts = slices.DeleteFunc(layouts, func(l ikLayout) bool { return l.name != "v26" && !slices.Contains(want, l.name) })
	}
	dir := filepath.Join(root, "variants")
	os.MkdirAll(dir, 0o755)
	base := map[string]uint64{}
	for _, l := range layouts {
		db := s.DB
		if l.name != "v26" {
			path := filepath.Join(dir, l.name+".db")
			if _, err := s.DB.Exec(`VACUUM INTO ?`, path); err != nil {
				t.Fatal(err)
			}
			db = ikOpen(t, path)
			db.Exec(`PRAGMA journal_mode=WAL`)
			conv, bst := ikConvert(t, db, l)
			db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
			pre := ikSizes(t, db)
			vb := time.Now()
			if _, err := db.Exec(`VACUUM`); err != nil {
				t.Fatal(err)
			}
			vac := time.Since(vb)
			db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
			t.Logf("CONVERT %s convert=%s file_before_vacuum=%.1fMB live_before_vacuum=%.1fMB vacuum=%s", l.name, conv.Round(time.Millisecond), float64(pre.file)/1e6, float64(pre.live)/1e6, vac.Round(time.Millisecond))
			if l.path != 0 && l.path != 4 {
				// cleanup of unreferenced paths, as maintenance would run it after pruning
				// manifests and fact sets (02 §7.2); everything is referenced here, so this
				// times the anti-join and deletes nothing
				refs := `SELECT path_id FROM snapshot_file UNION ALL SELECT file_id FROM def_fact WHERE file_id IS NOT NULL UNION ALL SELECT file_id FROM assembly_fact UNION ALL SELECT file_id FROM type_fact UNION ALL SELECT file_id FROM harmony_fact UNION ALL SELECT file_id FROM harmony_id`
				q := `DELETE FROM path WHERE id NOT IN (` + refs + `)`
				if l.path == 3 {
					q = `DELETE FROM mod_path WHERE (inst, id) NOT IN (SELECT k.n, f.path_id FROM snapshot_file f JOIN snapshot n ON n.id=f.snapshot_id JOIN instance_key k ON k.id=n.instance_id UNION ALL SELECT s.inst, f.file_id FROM def_fact f JOIN fact_set s ON s.id=f.set_id WHERE f.file_id IS NOT NULL UNION ALL SELECT s.inst, f.file_id FROM type_fact f JOIN fact_set s ON s.id=f.set_id UNION ALL SELECT s.inst, f.file_id FROM harmony_fact f JOIN fact_set s ON s.id=f.set_id UNION ALL SELECT s.inst, f.file_id FROM harmony_id f JOIN fact_set s ON s.id=f.set_id UNION ALL SELECT s.inst, f.file_id FROM assembly_fact f JOIN fact_set s ON s.id=f.set_id)`
				}
				gb := time.Now()
				tx, _ := db.Begin()
				res, err := tx.Exec(q)
				if err != nil {
					t.Fatal(err)
				}
				n, _ := res.RowsAffected()
				tx.Rollback()
				t.Logf("PATHGC %s %s deleted=%d", l.name, time.Since(gb).Round(time.Millisecond), n)
			}
			if l.blob != 0 {
				t.Logf("BLOB %s lists=%d stored_blobs=%d raw=%.1fMB raw+mtime=%.1fMB zstd=%.1fMB zstd+mtime=%.1fMB stored=%.1fMB", l.name, bst.lists, bst.blobs, float64(bst.raw)/1e6, float64(bst.rawMtime)/1e6, float64(bst.zstd)/1e6, float64(bst.zstdMtime)/1e6, float64(bst.stored)/1e6)
			}
		}
		ikLogSizes(t, db, l.name, ikSizes(t, db))
		t.Logf("DISK during %s: %d MB", l.name, ikDisk(t, limit)>>20)
		c := ikCtx{db: db, l: l, smp: smp}
		for _, j := range []string{"JOIN", "CROSS JOIN"} {
			col, _ := l.inst()
			plan, _ := ikLines(db.Query(`EXPLAIN QUERY PLAN `+c.defSelectJoin(j, `f.name=? AND f.name<>'' AND c.game_mm=?`, col+", f.rowid"), "x", "1.6"))
			t.Logf("PLAN %s Name %s: %s", l.name, j, strings.Join(plan, " ; "))
		}
		for _, q := range ikQueries() {
			lines, err := q.run(c) // warm-up, also the checksum
			if err != nil {
				t.Fatalf("%s %s: %v", l.name, q.name, err)
			}
			sum := ikHash(lines, q.ordered)
			same := "base"
			if l.name != "v26" {
				same = "same"
				if base[q.name] != sum {
					same = "DIFF"
				}
			} else {
				base[q.name] = sum
			}
			// repeat fast queries within a round: the Windows clock ticks in ~0.5 ms steps
			b := time.Now()
			if _, err := q.run(c); err != nil {
				t.Fatal(err)
			}
			reps := max(1, int(30*time.Millisecond/max(time.Microsecond, time.Since(b))))
			var ds []time.Duration
			for k := 0; k < rounds; k++ {
				b := time.Now()
				for j := 0; j < reps; j++ {
					if _, err := q.run(c); err != nil {
						t.Fatal(err)
					}
				}
				ds = append(ds, time.Since(b)/time.Duration(reps))
			}
			t.Logf("QUERY %s | %s | %s | rows=%d | %s", l.name, q.name, ikMedian(ds).Round(time.Microsecond), len(lines), same)
		}
		for _, w := range []struct {
			name string
			run  func() (time.Duration, error)
		}{
			{"write 30 snapshot manifests", func() (time.Duration, error) { return ikWriteSnapshots(c, smp.manifests) }},
			{"write 30 Def sets", func() (time.Duration, error) { return ikWriteDefSets(c, smp.defSets) }},
		} {
			var ds []time.Duration
			for k := 0; k < rounds+1; k++ {
				d, err := w.run()
				if err != nil {
					t.Fatalf("%s %s: %v", l.name, w.name, err)
				}
				if k > 0 {
					ds = append(ds, d)
				}
			}
			t.Logf("WRITE %s | %s | %s", l.name, w.name, ikMedian(ds).Round(100*time.Microsecond))
		}
		if l.name != "v26" {
			db.Close()
			for _, sfx := range []string{"", "-wal", "-shm"} {
				os.Remove(filepath.Join(dir, l.name+".db"+sfx))
			}
		}
	}
}
