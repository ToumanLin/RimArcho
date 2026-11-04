package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

// putSnap writes a snapshot with its file rows and derived columns, as insertSnapshot does.
func putSnap(t *testing.T, db *sql.DB, id int64, rels ...string) {
	t.Helper()
	k, l := deriveContent(rels)
	if _, err := db.Exec(`INSERT INTO snapshot(id, instance_id, scan_id, snapshot_hash, kinds, langs) VALUES(?, 'i', 1, x'00', ?, ?)`, id, k, l); err != nil {
		t.Fatal(err)
	}
	for _, r := range rels {
		if _, err := db.Exec(`INSERT INTO snapshot_file(snapshot_id, rel_path, size, mtime) VALUES(?, ?, 1, 1)`, id, r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestContentKindsScopedSnapshots(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// More than one query batch, shared snapshots, no snapshot, and paths whose
	// extensions alone must not imply Defs or Languages.
	putSnap(t, s.DB, 1, "1.6/Assemblies/code.DLL", "1.6/Defs/Things/thing.xml", "1.6/Languages/English/Keyed/key.xml", "Textures/test.png")
	for i := int64(2); i <= 505; i++ {
		putSnap(t, s.DB, i, "1.6/Assemblies/code.DLL")
	}
	putSnap(t, s.DB, 999, "other.xml", "Languages.txt", "texture.png")
	snaps := map[string]int64{"zero": 0, "absent": 700, "plain": 999, "shared": 1}
	for i := 1; i <= 505; i++ {
		snaps[fmt.Sprint("m", i)] = int64(i)
	}
	got, err := s.ContentKinds(snaps)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(snaps) || got["shared"] != HasDLL|HasDefsPatch|HasLanguages || got["zero"] != 0 || got["absent"] != 0 || got["plain"] != 0 {
		t.Fatalf("unexpected flags: %v", got)
	}
	for i := 2; i <= 505; i++ {
		if got[fmt.Sprint("m", i)] != HasDLL {
			t.Fatal("lost a snapshot across batches")
		}
	}
	png, err := s.SnapshotsWithPNGFor(map[string]int64{"yes": 1, "no": 999, "none": 0, "absent": 700})
	if err != nil || len(png) != 1 || !png[1] {
		t.Fatalf("PNG flags: %v, %v", png, err)
	}
	empty, err := s.ContentKinds(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty scope: %v, %v", empty, err)
	}
	if _, err := s.DB.Exec(`INSERT INTO instance(id, environment_id, lineage_id, source_id, rel_path, real_path, status, current_snapshot_id, first_seen, last_seen)
		VALUES('a', '', 'l', 's', 'a', 'a', 'live', 1, 1, 1), ('b', '', 'l', 's', 'b', 'b', 'live', 999, 1, 1), ('c', '', 'l', 's', 'c', 'c', 'live', NULL, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	all, err := s.SnapshotsWithPNG()
	if err != nil || !reflect.DeepEqual(all, map[int64]bool{1: true}) {
		t.Fatalf("SnapshotsWithPNG: %v, %v", all, err)
	}
}

// The flags stay with a snapshot after 02 §7.1 drops its file rows.
func TestContentKindsOutliveFileRows(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	putSnap(t, s.DB, 7, "Assemblies/a.dll", "Languages/English/Keyed/k.xml")
	if _, err := s.DB.Exec(`DELETE FROM snapshot_file WHERE snapshot_id=7`); err != nil {
		t.Fatal(err)
	}
	got, err := s.ContentKinds(map[string]int64{"i": 7})
	if err != nil || got["i"] != HasDLL|HasLanguages {
		t.Fatalf("flags %v, %v", got, err)
	}
	if ls, err := s.Languages(7); err != nil || !reflect.DeepEqual(ls, []string{"English"}) {
		t.Fatalf("languages %q, %v", ls, err)
	}
}
