package store

import (
	"database/sql"
	"github.com/ToumanLin/RimArcho/internal/analyze"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func v25Store(t *testing.T, path string) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	steps := []string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10, schemaV11, schemaV12, schemaV13, schemaV14, schemaV15, schemaV16, schemaV17, schemaV18, schemaV19, schemaV20, schemaV21, schemaV22, schemaV23, schemaV24, schemaV25}
	if _, err := db.Exec(strings.Join(steps, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO meta VALUES('schema_version','25')`); err != nil {
		t.Fatal(err)
	}
	return &Store{DB: db, Path: path}
}

func TestMigrateV26FactViews(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	s := v25Store(t, path)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, mm := range []string{"1.6", "1.7"} {
		for _, inst := range []string{"shared", "different", "version", "snapshot", "empty"} {
			folder := "C:/Synthetic/" + inst
			version := "a8"
			hash := "0102"
			if inst == "different" {
				folder += "/" + mm
			}
			if inst == "version" && mm == "1.7" {
				version = "a7"
			}
			if inst == "snapshot" && mm == "1.7" {
				hash = "0304"
			}
			exec(`INSERT INTO content_state(instance_id,game_mm,content_key,folders,def_files,patch_files,parse_errors) VALUES(?,?,?,?,2,3,'parse error')`, inst, mm, hash+"|"+mm+"|condition="+mm+"|"+version, folder)
			if inst == "empty" {
				continue
			}
			exec(`INSERT INTO def_fact(instance_id,game_mm,def_type,def_name,name,parent,abstract,file,line,may_require,may_require_any) VALUES(?,?,'ThingDef','Gun','Base','Parent',1,'Defs/A.xml',23,'all.pkg','any.pkg')`, inst, mm)
		}
		exec(`INSERT INTO assembly_fact(instance_id,game_mm,file,skipped,manual,uses_harmony,types,patches,error) VALUES('shared',?,'Assemblies/A.dll',0,1,1,1,1,'')`, mm)
		exec(`INSERT INTO type_fact(instance_id,game_mm,file,type_name) VALUES('shared',?,'Assemblies/A.dll','Synthetic.Outer+Inner')`, mm)
		exec(`INSERT INTO harmony_fact(instance_id,game_mm,file,patch_type,patch_method,kind,target,target_type,target_method,args,skips,dynamic) VALUES('shared',?,'Assemblies/A.dll','Synthetic.Patch','Prefix','prefix','Verse.Pawn.Tick()','Verse.Pawn','Tick','',1,2)`, mm)
		exec(`INSERT INTO harmony_id(instance_id,game_mm,file,id) VALUES('shared',?,'Assemblies/A.dll','Synthetic.ID')`, mm)
	}
	exec(`INSERT INTO def_fact(instance_id,game_mm,def_type,def_name,file,line) VALUES('orphan','1.6','ThingDef','Unused','A.xml',1)`)
	rows, err := s.DB.Query(`SELECT instance_id,def_type,def_name,name,parent,abstract,file,line,may_require,may_require_any FROM def_fact WHERE game_mm='1.6' AND def_name='Gun' ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	before, err := scanDefs(rows)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(before, func(a, b analyze.DefFact) int { return strings.Compare(a.Instance, b.Instance) })
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, mm := range []string{"1.6", "1.7"} {
		s.MM = mm
		fs, err := s.DefsNamed("Gun", false)
		slices.SortFunc(fs, func(a, b analyze.DefFact) int { return strings.Compare(a.Instance, b.Instance) })
		if err != nil || !reflect.DeepEqual(before, fs) {
			t.Fatalf("%s defs: %v, want %v (%v)", mm, fs, before, err)
		}
		named, err := s.DefsNamed("Base", true)
		slices.SortFunc(named, func(a, b analyze.DefFact) int { return strings.Compare(a.Instance, b.Instance) })
		if err != nil || !reflect.DeepEqual(fs, named) {
			t.Fatal("Name view", err)
		}
		asm, err := s.Assemblies("shared")
		if err != nil || len(asm) != 1 || !asm[0].Manual || !asm[0].UsesHm || asm[0].Types != 1 {
			t.Fatalf("assemblies %v %v", asm, err)
		}
		owners, err := s.TypeOwners("Synthetic.Outer.Inner")
		if err != nil || !reflect.DeepEqual(owners, [][2]string{{"shared", "Assemblies/A.dll"}}) {
			t.Fatal("type owners", owners, err)
		}
		ids, err := s.HarmonyIDOwners("synthetic.id")
		if err != nil || !reflect.DeepEqual(ids, []string{"shared"}) {
			t.Fatal("ID owners", ids, err)
		}
		patches, err := s.HarmonyPatches(HarmonyFilter{Instance: "shared", Target: "Verse.Pawn.Tick"})
		if err != nil || len(patches) != 1 || !patches[0].Manual || !patches[0].Skips {
			t.Fatal("patches", patches, err)
		}
		key := s.ContentKey("shared")
		cached, ok := s.CachedDefs("shared", key)
		if !ok || len(cached) != 1 {
			t.Fatal("converted Def cache missed")
		}
		empty, ok := s.CachedDefs("empty", s.ContentKey("empty"))
		if !ok || len(empty) != 0 {
			t.Fatal("empty Def cache missed")
		}
	}
	for inst, want := range map[string]string{"shared": "1", "different": "2", "version": "2", "snapshot": "2", "empty": "1"} {
		if got := q1(t, s, `SELECT count(*) FROM fact_set WHERE instance_id=? AND kind='def'`, inst); got != want {
			t.Fatalf("%s: %s sets, want %s", inst, got, want)
		}
	}
	if got := q1(t, s, `SELECT count(*) FROM assembly_fact`); got != "1" {
		t.Fatal("identical DLL facts duplicated", got)
	}
	if got := q1(t, s, `SELECT count(*) FROM def_fact WHERE def_name='Unused'`); got != "0" {
		t.Fatal("orphan facts migrated", got)
	}
	if got := q1(t, s, `SELECT count(*) FROM pragma_foreign_key_check`); got != "0" {
		t.Fatal("foreign keys", got)
	}
}

func TestMigrateV26Scale(t *testing.T) {
	if os.Getenv("RIMARCHO_FACT_BENCH") == "" {
		t.Skip("set RIMARCHO_FACT_BENCH=1 for migration scale measurement")
	}
	path := filepath.Join(t.TempDir(), "legacy.db")
	s := v25Store(t, path)
	exec := func(q string) {
		t.Helper()
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE synthetic_mod(n INTEGER PRIMARY KEY); WITH RECURSIVE n(x) AS(VALUES(0) UNION ALL SELECT x+1 FROM n WHERE x<3999) INSERT INTO synthetic_mod SELECT x FROM n;
 CREATE TABLE synthetic_def(n INTEGER PRIMARY KEY); WITH RECURSIVE n(x) AS(VALUES(0) UNION ALL SELECT x+1 FROM n WHERE x<94) INSERT INTO synthetic_def SELECT x FROM n;
 CREATE TABLE synthetic_mm(mm TEXT PRIMARY KEY); INSERT INTO synthetic_mm VALUES('1.6'),('1.7');`)
	exec(`INSERT INTO content_state(instance_id,game_mm,content_key,folders) SELECT printf('synthetic-instance-%08d',n),mm,printf('%08x',n)||'|'||mm||'||a8',printf('C:/Synthetic/mod-%04d',n) FROM synthetic_mod CROSS JOIN synthetic_mm;`)
	exec(`INSERT INTO def_fact(instance_id,game_mm,def_type,def_name,name,file,line) SELECT printf('synthetic-instance-%08d',m.n),mm,'ThingDef',printf('Def_%04d_%03d',m.n,d.n),CASE WHEN d.n%20=0 THEN printf('Parent_%04d_%03d',m.n,d.n) ELSE '' END,'Defs/Synthetic-content-definitions.xml',1 FROM synthetic_mod m CROSS JOIN synthetic_def d CROSS JOIN synthetic_mm;`)
	exec(`INSERT INTO assembly_fact(instance_id,game_mm,file,types) SELECT instance_id,game_mm,'Assemblies/Synthetic.dll',20 FROM content_state;`)
	exec(`INSERT INTO type_fact(instance_id,game_mm,file,type_name) SELECT instance_id,game_mm,'Assemblies/Synthetic.dll',printf('Synthetic.Type_%s_%d',instance_id,d.n) FROM content_state CROSS JOIN synthetic_def d WHERE d.n<20;`)
	exec(`DROP TABLE synthetic_mod; DROP TABLE synthetic_def; DROP TABLE synthetic_mm; VACUUM;`)
	var p, z int64
	s.DB.QueryRow(`PRAGMA page_count`).Scan(&p)
	s.DB.QueryRow(`PRAGMA page_size`).Scan(&z)
	t.Logf("MIGRATION before=%d bytes", p*z)
	s.Close()
	begin := time.Now()
	var err error
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t.Logf("MIGRATION elapsed=%s", time.Since(begin))
	var free int64
	s.DB.QueryRow(`PRAGMA page_count`).Scan(&p)
	s.DB.QueryRow(`PRAGMA freelist_count`).Scan(&free)
	t.Logf("MIGRATION after live=%d physical=%d free=%d bytes", (p-free)*z, p*z, free*z)
	if got := q1(t, s, `SELECT count(*) FROM def_fact`); got != "380000" {
		t.Fatal(got)
	}
	if got := q1(t, s, `SELECT count(*) FROM type_fact`); got != "80000" {
		t.Fatal(got)
	}
	s.MM = "1.6"
	fs, err := s.DefsNamed("Parent_0001_000", true)
	if err != nil || len(fs) != 1 {
		t.Fatal(fs, err)
	}
	s.MM = "1.7"
	other, err := s.DefsNamed("Parent_0001_000", true)
	if err != nil || !reflect.DeepEqual(fs, other) {
		t.Fatal(other, err)
	}
	// Compaction belongs to maintenance, not the migration's startup transaction.
	exec(`PRAGMA wal_checkpoint(TRUNCATE); VACUUM;`)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MIGRATION compacted=%d bytes", info.Size())
	if len(fs) > 0 && fs[0].Type != "ThingDef" {
		t.Fatalf("bad migrated fact: %+v", fs[0])
	}
}
