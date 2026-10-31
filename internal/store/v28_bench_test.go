package store_test

// TestV28Formal measures the formal v28 layout (02 §7, 03 §3) on TestIntKeyResearch's
// synthetic library: the library is written in the v27 layout, timed with the v27 queries,
// copied, upgraded by store.Open and timed again with the Store's own methods. Opt-in, with
// TMP/TEMP on the scratch folder:
//
//	$env:RIMARCHO_INTKEY_BENCH = '1'; $env:RIMARCHO_INTKEY_DISK_ROOT = 'D:\RimArcho-test\db-v28'
//	go test ./internal/store -run '^TestV28Formal$' -v -count=1 -timeout 60m

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestV28Formal(t *testing.T) {
	if os.Getenv("RIMARCHO_INTKEY_BENCH") == "" {
		t.Skip("set RIMARCHO_INTKEY_BENCH=1 for the v28 benchmark")
	}
	scale := 1.0
	if v := os.Getenv("RIMARCHO_INTKEY_SCALE"); v != "" {
		scale, _ = strconv.ParseFloat(v, 64)
	}
	limit := int64(1400) << 20
	rounds := 3
	dir := t.TempDir()
	const envID = "bench-env"
	db := store.NewV27DB(t, filepath.Join(dir, "v27.db"))
	if _, err := db.Exec(`INSERT INTO environment(id,name,game_root,game_version,created_at) VALUES(?,?,?,?,1)`, envID, "bench", `D:\bench`, "1.6.4633"); err != nil {
		t.Fatal(err)
	}
	begin := time.Now()
	smp := ikLoad(t, db, ikPlanLibrary(scale, envID), envID)
	t.Logf("BUILD %s", time.Since(begin).Round(time.Millisecond))
	ls, err := ikLines(db.Query(`SELECT c.instance_id, c.content_key FROM content_state c WHERE c.game_mm='1.6' AND c.instance_id NOT IN (SELECT id FROM bench_active) ORDER BY c.instance_id`))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range ls {
		a, b, _ := strings.Cut(l, "\x1f")
		smp.cached = append(smp.cached, [2]string{a, b})
	}
	r := rand.New(rand.NewPCG(9, 9))
	for k := 0; k < 30; k++ {
		snap := smp.snaps[r.IntN(len(smp.snaps))]
		var inst string
		db.QueryRow(`SELECT instance_id FROM snapshot WHERE id=?`, snap).Scan(&inst)
		rows, err := db.Query(`SELECT rel_path, size, mtime, hash FROM snapshot_file WHERE snapshot_id=? ORDER BY rel_path`, snap)
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
	for _, q := range []string{`PRAGMA wal_checkpoint(TRUNCATE)`, `VACUUM`, `PRAGMA wal_checkpoint(TRUNCATE)`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	ikLogSizes(t, db, "v27", ikSizes(t, db))
	t.Logf("DISK after build: %d MB", ikDisk(t, limit)>>20)

	timeIt := func(run func() (int, error)) (time.Duration, int) {
		t.Helper()
		n, err := run() // warm-up
		if err != nil {
			t.Fatal(err)
		}
		b := time.Now()
		if _, err := run(); err != nil {
			t.Fatal(err)
		}
		reps := max(1, int(30*time.Millisecond/max(time.Microsecond, time.Since(b))))
		var ds []time.Duration
		for k := 0; k < rounds; k++ {
			b := time.Now()
			for j := 0; j < reps; j++ {
				if _, err := run(); err != nil {
					t.Fatal(err)
				}
			}
			ds = append(ds, time.Since(b)/time.Duration(reps))
		}
		return ikMedian(ds), n
	}

	// v27: the research's baseline SQL (layout "v26" is the v27 layout without the derived columns' readers).
	old := ikCtx{db: db, l: ikLayout{name: "v26"}, smp: smp}
	oldQ := map[string]ikQuery{}
	for _, q := range ikQueries() {
		oldQ[q.name] = q
	}
	type pair struct{ old, formal string }
	pairs := []pair{
		{"defName×100(CROSS JOIN)", "DefsNamed defName×100"},
		{"Name×190(CROSS JOIN)", "DefsNamed Name×190"},
		{"DefsOf×100", "DefsOf×100"},
		{"CachedDefs(未启用全部)", "CachedDefs(未启用全部)"},
		{"TypeOwners×100", "TypeOwners×100"},
		{"HarmonyPatches(全部)", "HarmonyPatches(全部)"},
		{"CurrentFiles(增量扫描)", "CurrentFiles(增量扫描)"},
		{"SnapshotFiles×100(变更页)", "SnapshotFiles×100(变更页)"},
		{"InstanceFiles(%Textures/%)", "InstanceFiles(%Textures/%)"},
		{"InstanceFiles(%x.dll)×20", "InstanceFiles(%x.dll)×20"},
		{"ContentKinds(全部,无缓存)", "ContentKinds(全部)"},
	}
	// Mod 库存's first open reads snapshot.kinds by primary key, the same SQL in v27 and v28.
	oldQ["kinds"] = ikQuery{name: "kinds", run: func(c ikCtx) ([]string, error) {
		var all []string
		ids := c.smp.snaps
		for len(ids) > 0 {
			n := min(len(ids), 500)
			args := make([]any, n)
			for i := range args {
				args[i] = ids[i]
			}
			ls, err := ikLines(c.db.Query(`SELECT id, kinds FROM snapshot WHERE id IN (?`+strings.Repeat(",?", n-1)+`)`, args...))
			if err != nil {
				return nil, err
			}
			all = append(all, ls...)
			ids = ids[n:]
		}
		return all, nil
	}}
	pairs[len(pairs)-1].old = "kinds"
	oldTimes, oldRows := map[string]time.Duration{}, map[string]int{}
	for _, p := range pairs {
		q := oldQ[p.old]
		d, n := timeIt(func() (int, error) { ls, err := q.run(old); return len(ls), err })
		oldTimes[p.formal], oldRows[p.formal] = d, n
	}
	writeOld, _ := timeIt(func() (int, error) { d, err := ikWriteSnapshots(old, smp.manifests); _ = d; return 0, err })

	// v28: copy, upgrade with store.Open, then the Store's methods.
	path := filepath.Join(dir, "v28.db")
	if _, err := db.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	db.Close()
	os.Remove(filepath.Join(dir, "v27.db"))
	os.Remove(filepath.Join(dir, "v27.db-wal"))
	var last store.UpgradeProgress
	var calls int
	begin = time.Now()
	s, err := store.OpenUpgrading(path, func(p store.UpgradeProgress) { last = p; calls++ })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	upgrade := time.Since(begin)
	pre := ikSizes(t, s.DB)
	walSize := int64(0)
	if fi, err := os.Stat(path + "-wal"); err == nil {
		walSize = fi.Size()
	}
	t.Logf("DISK during upgrade: %d MB", ikDisk(t, limit)>>20)
	s.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	vb := time.Now()
	if _, err := s.DB.Exec(`VACUUM`); err != nil {
		t.Fatal(err)
	}
	vac := time.Since(vb)
	s.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	t.Logf("UPGRADE v27→v28 %s (progress calls %d, last %d/%d) file_before_vacuum=%.1fMB live_before_vacuum=%.1fMB wal=%.1fMB vacuum=%s",
		upgrade.Round(time.Millisecond), calls, last.Done, last.Total, float64(pre.file)/1e6, float64(pre.live)/1e6, float64(walSize)/1e6, vac.Round(time.Millisecond))
	ikLogSizes(t, s.DB, "v28", ikSizes(t, s.DB))
	var blocks, entries, bytes int64
	s.DB.QueryRow(`SELECT count(*), coalesce(sum(n),0), coalesce(sum(length(data)),0) FROM snapshot_list`).Scan(&blocks, &entries, &bytes)
	t.Logf("BLOCKS %d blocks, %d files, %.2f MB", blocks, entries, float64(bytes)/1e6)

	s.MM = "1.6"
	snapOf := map[string]int64{}
	rows, err := s.DB.Query(`SELECT id, current_snapshot_id FROM instance WHERE current_snapshot_id IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		var snap int64
		rows.Scan(&id, &snap)
		snapOf[id] = snap
	}
	rows.Close()
	count := func(m map[string][]string) int {
		n := 0
		for _, l := range m {
			n += len(l)
		}
		return n
	}
	formal := map[string]func() (int, error){
		"DefsNamed defName×100": func() (int, error) {
			n := 0
			for _, x := range smp.defNames[:min(100, len(smp.defNames))] {
				fs, err := s.DefsNamed(x, false)
				if err != nil {
					return 0, err
				}
				n += len(fs)
			}
			return n, nil
		},
		"DefsNamed Name×190": func() (int, error) {
			n := 0
			for _, x := range smp.names[:min(190, len(smp.names))] {
				fs, err := s.DefsNamed(x, true)
				if err != nil {
					return 0, err
				}
				n += len(fs)
			}
			return n, nil
		},
		"DefsOf×100": func() (int, error) {
			n := 0
			for _, id := range smp.insts[:min(100, len(smp.insts))] {
				fs, err := s.DefsOf(id)
				if err != nil {
					return 0, err
				}
				n += len(fs)
			}
			return n, nil
		},
		"CachedDefs(未启用全部)": func() (int, error) {
			n := 0
			for _, k := range smp.cached {
				fs, ok := s.CachedDefs(k[0], k[1])
				if !ok {
					return 0, fmt.Errorf("cached defs of %s missed", k[0])
				}
				n += len(fs)
			}
			return n, nil
		},
		"TypeOwners×100": func() (int, error) {
			n := 0
			for _, x := range smp.types[:min(100, len(smp.types))] {
				ps, err := s.TypeOwners(x)
				if err != nil {
					return 0, err
				}
				n += len(ps)
			}
			return n, nil
		},
		"HarmonyPatches(全部)": func() (int, error) {
			hs, err := s.HarmonyPatches(store.HarmonyFilter{})
			return len(hs), err
		},
		"CurrentFiles(增量扫描)": func() (int, error) {
			m, err := s.CurrentFiles(envID)
			n := 0
			for _, x := range m {
				n += len(x)
			}
			return n, err
		},
		"SnapshotFiles×100(变更页)": func() (int, error) {
			n := 0
			for _, snap := range smp.snaps[:min(100, len(smp.snaps))] {
				fs, err := s.SnapshotFiles(snap)
				if err != nil {
					return 0, err
				}
				n += len(fs)
			}
			return n, nil
		},
		"InstanceFiles(%Textures/%)": func() (int, error) {
			m, err := s.InstanceFiles(envID, "%Textures/%")
			return count(m), err
		},
		"InstanceFiles(%x.dll)×20": func() (int, error) {
			n := 0
			for _, x := range smp.dllNames[:min(20, len(smp.dllNames))] {
				m, err := s.InstanceFiles(envID, "%"+x)
				if err != nil {
					return 0, err
				}
				n += count(m)
			}
			return n, nil
		},
		"ContentKinds(全部)": func() (int, error) {
			m, err := s.ContentKinds(snapOf)
			return len(m), err
		},
	}
	for _, p := range pairs {
		d, n := timeIt(formal[p.formal])
		t.Logf("QUERY %-28s | v27 %10s rows=%-7d | v28 %10s rows=%-7d | %.2fx", p.formal, oldTimes[p.formal].Round(time.Microsecond), oldRows[p.formal], d.Round(time.Microsecond), n, float64(d)/float64(max(1, oldTimes[p.formal])))
	}
	cold, _ := timeIt(func() (int, error) {
		n := 0
		for _, x := range smp.names[:min(190, len(smp.names))] {
			s.DropSetFileCache()
			fs, err := s.DefsNamed(x, true)
			if err != nil {
				return 0, err
			}
			n += len(fs)
		}
		return n, nil
	})
	t.Logf("QUERY %-28s | v28 without the set-list cache %s", "DefsNamed Name×190", cold.Round(time.Microsecond))
	writeNew, _ := timeIt(func() (int, error) {
		tx, err := s.DB.Begin()
		if err != nil {
			return 0, err
		}
		defer tx.Rollback()
		for k, m := range smp.manifests {
			res, err := tx.Exec(`INSERT INTO snapshot(instance_id,scan_id,snapshot_hash) VALUES(?,1,?)`, m.inst, []byte(fmt.Sprint("w", k)))
			if err != nil {
				return 0, err
			}
			id, _ := res.LastInsertId()
			files := make([]scan.File, len(m.rows))
			for i, x := range m.rows {
				files[i] = scan.File{Rel: x.rel, Size: x.size, MTime: x.mtime, HasHash: x.hash != nil}
				copy(files[i].Hash[:], x.hash)
			}
			if err := store.InsertFilesTx(tx, id, files); err != nil {
				return 0, err
			}
		}
		return 0, nil
	})
	t.Logf("WRITE 30 snapshot manifests | v27 %s | v28 %s", writeOld.Round(100*time.Microsecond), writeNew.Round(100*time.Microsecond))
	t.Logf("DISK at end: %d MB", ikDisk(t, limit)>>20)
}
