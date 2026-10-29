package testkit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// MaintenanceHistory seeds synthetic update history without reading game files.
// Long paths allow the opt-in disk benchmark to reach its target with bounded rows.
func MaintenanceHistory(t testing.TB, s *store.Store, instance string, versions, files, pathBytes int) []int64 {
	t.Helper()
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	exec := func(q string, args ...any) int64 {
		r, err := tx.Exec(q, args...)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := r.LastInsertId()
		return id
	}
	exec(`INSERT INTO instance(id,environment_id,lineage_id,source_id,rel_path,status,first_seen,last_seen) VALUES(?,'',?,'',?,'live',?,?)`, instance, instance, instance, Start.Unix(), Start.Unix())
	exec(`INSERT INTO lineage(id,environment_id,created_at) VALUES(?,'',?)`, instance, Start.Unix())
	var ids []int64
	for v := 0; v < versions; v++ {
		scan := exec(`INSERT INTO scan(environment_id,started_at,finished_at) VALUES('',?,?)`, Start.Unix()+int64(v), Start.Unix()+int64(v))
		id := exec(`INSERT INTO snapshot(instance_id,scan_id,snapshot_hash,file_count,semantic_count,total_size) VALUES(?,?,?, ?,?,?)`, instance, scan, []byte{byte(v)}, files, (files+3)/4, files*100)
		exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<?)
		 INSERT INTO snapshot_file(snapshot_id,rel_path,size,mtime,hash)
		 SELECT ?, printf('%s/%08d.xml', ?,x),100,?,CASE WHEN x%4=1 THEN ? ELSE NULL END FROM n`, files, id, strings.Repeat("p", max(1, pathBytes)), v, []byte(fmt.Sprint(v)))
		if v > 0 {
			exec(`INSERT INTO change_event(scan_id,instance_id,kind,before_snapshot,after_snapshot) VALUES(?,?,'updated',?,?)`, scan, instance, ids[v-1], id)
		}
		ids = append(ids, id)
	}
	exec(`UPDATE instance SET current_snapshot_id=? WHERE id=?`, ids[len(ids)-1], instance)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return ids
}
