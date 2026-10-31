package store

import (
	"path/filepath"
	"testing"
)

func TestContentKindsScopedSnapshots(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// More than one query batch, shared snapshots, no snapshot, and paths whose
	// extensions alone must not imply Defs or Languages.
	_, err = s.DB.Exec(`WITH RECURSIVE ids(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM ids WHERE n<505)
 INSERT INTO snapshot_file(snapshot_id,rel_path,size,mtime) SELECT n,'1.6/Assemblies/code.DLL',1,1 FROM ids;
 INSERT INTO snapshot_file VALUES(1,'1.6/Defs/Things/thing.xml',1,1,NULL),(1,'1.6/Languages/English/Keyed/key.xml',1,1,NULL),
 (999,'other.xml',1,1,NULL),(999,'Languages.txt',1,1,NULL),(999,'texture.png',1,1,NULL),(1,'Textures/test.png',1,1,NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	snaps := map[string]int64{"zero": 0, "absent": 700, "plain": 999, "shared": 1}
	for i := 1; i <= 505; i++ {
		snaps[string(rune(i+1000))] = int64(i)
	}
	got, err := s.ContentKinds(snaps)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(snaps) || got["shared"] != HasDLL|HasDefsPatch|HasLanguages || got["zero"] != 0 || got["absent"] != 0 || got["plain"] != 0 {
		t.Fatalf("unexpected flags: %v", got)
	}
	for i := 2; i <= 505; i++ {
		if got[string(rune(i+1000))] != HasDLL {
			t.Fatal("lost a snapshot across batches")
		}
	}
	png, err := s.SnapshotsWithPNGFor(map[string]int64{"yes": 1, "no": 999, "none": 0})
	if err != nil || len(png) != 1 || !png[1] {
		t.Fatalf("PNG flags: %v, %v", png, err)
	}
	empty, err := s.ContentKinds(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty scope: %v, %v", empty, err)
	}
}

// A snapshot's flags are kept, but an ID that gc freed and a later scan reused is read again.
func TestContentKindsCacheFollowsReusedIDs(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	put := func(scan int, file string) {
		t.Helper()
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`DELETE FROM snapshot_file WHERE snapshot_id=7`, nil},
			{`DELETE FROM snapshot WHERE id=7`, nil},
			{`INSERT INTO snapshot(id, instance_id, scan_id, snapshot_hash) VALUES(7, 'i', ?, x'00')`, []any{scan}},
			{`INSERT INTO snapshot_file(snapshot_id, rel_path, size, mtime) VALUES(7, ?, 1, 1)`, []any{file}},
		} {
			if _, err := s.DB.Exec(q.sql, q.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	kinds := func() int {
		t.Helper()
		got, err := s.ContentKinds(map[string]int64{"i": 7})
		if err != nil {
			t.Fatal(err)
		}
		return got["i"]
	}
	put(1, "Assemblies/a.dll")
	if first, again := kinds(), kinds(); first != HasDLL || again != HasDLL {
		t.Fatal("first read")
	}
	put(2, "Languages/English/k.xml")
	if got := kinds(); got != HasLanguages {
		t.Fatalf("reused ID kept the old flags: %d", got)
	}
}
