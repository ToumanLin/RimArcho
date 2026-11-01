package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateSourceLibraryHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Restore the v28 shape, including a successful and a failed attempt.
	_, err = s.DB.Exec(`DROP TABLE source_library;
	ALTER TABLE source_use DROP COLUMN mode;
	ALTER TABLE source_use RENAME TO source_cache;
	INSERT INTO source_cache(dll_hash,instance_id,file,status,error,decompiled_at) VALUES
	('oldhash','copy-a','Assemblies/A.dll','ok','',100),
	('oldhash','copy-b','Assemblies/A.dll','error','broken assembly',200);
	UPDATE meta SET value='28' WHERE key='schema_version'`)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, id := range []string{"copy-a", "copy-b"} {
		rs, err := s.DecompileRecords(id)
		if err != nil || len(rs) != 1 || rs[0].Hash != "oldhash" {
			t.Fatalf("history %s: %+v %v", id, rs, err)
		}
		if id == "copy-b" && (rs[0].Err != "broken assembly" || rs[0].At.Unix() != 200) {
			t.Fatalf("failed attempt lost: %+v", rs[0])
		}
	}
	if got := q1(t, s, `SELECT count(*) FROM source_library`); got != "0" {
		t.Fatal("migration invented generated output", got)
	}
	if got := q1(t, s, `SELECT count(*) FROM sqlite_master WHERE name='source_cache'`); got != "0" {
		t.Fatal("old table retained", got)
	}
	if got := q1(t, s, `SELECT mode FROM source_use WHERE instance_id='copy-a'`); got != "project" {
		t.Fatal(got)
	}
	if err := s.RecordSourceUse("oldhash", "copy-b", "Assemblies/A.dll", "type-hit", "ok", "", time.Unix(300, 0)); err != nil {
		t.Fatal(err)
	}
	if got := q1(t, s, `SELECT count(*) FROM source_use`); got != "2" {
		t.Fatal("reuse duplicated history", got)
	}
}
