// Spike 4: process memory of modernc.org/sqlite after writing 500 mods' snapshots and
// file manifests. Uses the real mod folders for realistic file counts and path lengths
// (metadata only: paths, sizes, mtimes; hashes are synthetic so no file is read).
//
//	go run ./spikes/sqlitemem -mods 500
package main

import (
	"crypto/rand"
	"database/sql"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/rimarcho/rimarcho/spikes/internal/procmem"
	"github.com/rimarcho/rimarcho/spikes/internal/rwenv"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE instance (id INTEGER PRIMARY KEY, package_id TEXT, name TEXT, root TEXT, source TEXT);
CREATE TABLE snapshot (id INTEGER PRIMARY KEY, instance_id INTEGER REFERENCES instance(id),
  taken_at INTEGER, fingerprint BLOB, about_xml TEXT, file_count INTEGER, total_bytes INTEGER);
CREATE TABLE manifest_file (snapshot_id INTEGER REFERENCES snapshot(id), rel_path TEXT, size INTEGER,
  mtime INTEGER, hash BLOB, PRIMARY KEY (snapshot_id, rel_path)) WITHOUT ROWID;
CREATE INDEX instance_pkg ON instance(package_id);
`

func main() {
	nMods := flag.Int("mods", 500, "number of mods to write")
	dbPath := flag.String("db", filepath.Join(os.TempDir(), "rimarcho-sqlitemem.db"), "database file")
	cacheKB := flag.Int("cache_kb", 2000, "PRAGMA cache_size in KiB (sqlite default ≈ 2000)")
	journal := flag.String("journal", "WAL", "PRAGMA journal_mode")
	mmapMB := flag.Int("mmap_mb", 0, "PRAGMA mmap_size in MiB")
	flag.Parse()
	os.Remove(*dbPath)
	os.Remove(*dbPath + "-wal")
	os.Remove(*dbPath + "-shm")

	report("start")
	env, err := rwenv.Detect()
	must(err)
	reg := env.Scan()
	mods := reg.All
	if len(mods) > *nMods {
		mods = mods[:*nMods]
	}

	db, err := sql.Open("sqlite", *dbPath)
	must(err)
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode=" + *journal, "PRAGMA synchronous=NORMAL",
		fmt.Sprintf("PRAGMA mmap_size=%d", *mmapMB<<20),
		fmt.Sprintf("PRAGMA cache_size=-%d", *cacheKB), "PRAGMA foreign_keys=ON"} {
		_, err := db.Exec(p)
		must(err)
	}
	_, err = db.Exec(schema)
	must(err)
	report("opened")

	t0 := time.Now()
	files, bytes := 0, int64(0)
	tx, err := db.Begin()
	must(err)
	insInst, _ := tx.Prepare(`INSERT INTO instance(package_id,name,root,source) VALUES(?,?,?,?)`)
	insSnap, _ := tx.Prepare(`INSERT INTO snapshot(instance_id,taken_at,fingerprint,about_xml,file_count,total_bytes) VALUES(?,?,?,?,?,?)`)
	insFile, _ := tx.Prepare(`INSERT INTO manifest_file(snapshot_id,rel_path,size,mtime,hash) VALUES(?,?,?,?,?)`)
	for _, m := range mods {
		res, err := insInst.Exec(m.PackageID, m.Name, m.Dir, m.Source.String())
		must(err)
		instID, _ := res.LastInsertId()
		about, _ := os.ReadFile(filepath.Join(m.Dir, "About", "About.xml"))
		fp := make([]byte, 16)
		rand.Read(fp)
		res, err = insSnap.Exec(instID, time.Now().Unix(), fp, string(about), 0, 0)
		must(err)
		snapID, _ := res.LastInsertId()
		n, total := 0, int64(0)
		filepath.WalkDir(m.Dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(m.Dir, p)
			h := make([]byte, 16) // xxh3-128 sized
			rand.Read(h)
			_, err = insFile.Exec(snapID, filepath.ToSlash(rel), info.Size(), info.ModTime().Unix(), h)
			must(err)
			n++
			total += info.Size()
			return nil
		})
		_, err = tx.Exec(`UPDATE snapshot SET file_count=?, total_bytes=? WHERE id=?`, n, total, snapID)
		must(err)
		files += n
		bytes += total
	}
	must(tx.Commit())
	el := time.Since(t0)
	fmt.Printf("wrote %d mods, %d manifest rows (%.1f GB of mod files) in %v (%.0f rows/s)\n",
		len(mods), files, float64(bytes)/1e9, el.Round(time.Millisecond), float64(files)/el.Seconds())
	report("after insert")

	// A typical read: list view query.
	t1 := time.Now()
	rows, err := db.Query(`SELECT i.package_id, i.name, s.file_count, s.total_bytes FROM instance i JOIN snapshot s ON s.instance_id=i.id ORDER BY i.name`)
	must(err)
	cnt := 0
	for rows.Next() {
		var a, b string
		var c, d int64
		rows.Scan(&a, &b, &c, &d)
		cnt++
	}
	rows.Close()
	fmt.Printf("list query: %d rows in %v\n", cnt, time.Since(t1).Round(time.Microsecond))
	report("after query")
	runtime.GC()
	debug.FreeOSMemory()
	report("after GC+FreeOSMemory")
	if st, err := os.Stat(*dbPath); err == nil {
		wal, _ := os.Stat(*dbPath + "-wal")
		var walSize int64
		if wal != nil {
			walSize = wal.Size()
		}
		fmt.Printf("db file: %.1f MB, wal: %.1f MB\n", float64(st.Size())/1e6, float64(walSize)/1e6)
	}
	db.Close()
}

func report(stage string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	ws, private, _ := procmem.MB()
	fmt.Printf("[%-22s] working set %6.1f MB  private %6.1f MB  go heap in use %6.1f MB  go sys %6.1f MB\n",
		stage, ws, private, float64(m.HeapInuse)/(1<<20), float64(m.Sys)/(1<<20))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
