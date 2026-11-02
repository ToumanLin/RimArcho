package store

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/analyze"
)

// v27Store creates a database as a v27 build did.
func v27Store(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	steps := []string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10, schemaV11, schemaV12, schemaV13, schemaV14, schemaV15, schemaV16, schemaV17, schemaV18, schemaV19, schemaV20, schemaV21, schemaV22, schemaV23, schemaV24, schemaV25, schemaV26, schemaV27}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(strings.Join(steps, "")); err != nil {
		t.Fatal(err)
	}
	if err := migrateV26(tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', '27')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return db
}

// v27Fill writes synthetic snapshots with mixed manifests and fact sets in the v27 layout.
func v27Fill(t *testing.T, db *sql.DB) {
	t.Helper()
	r := rand.New(rand.NewPCG(28, 28))
	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	pool := []string{"About/About.xml", "About/Preview.png", "LoadFolders.xml", "Defs/ThingDefs/A.xml", "1.6/Defs/B.xml", "Patches/P.xml",
		"Assemblies/Mod.dll", "Assemblies/0Harmony.dll", "Libs/native.DLL", "Plugins/x_y.dll", "Textures/Things/A/a_north.png",
		"Textures/Things/A/A_SOUTH.PNG", "textures/ui/icon.png", "1.6/Textures/b.png", "Textures/中文/图.png", "Textures/100%/c.png",
		"Sounds/click.ogg", "Languages/English/Keyed/A.xml", "Languages/ChineseSimplified (简体中文)/Keyed/A.xml", "1.6/Languages/German/Keyed/x.xml",
		"News/readme.txt", "Textures/a.png.meta", "Defs/Readme.txt", "Assemblies/Mod.pdb", "Textures/Things/A/a_east.png"}
	semantic := func(rel string) bool {
		l := strings.ToLower(rel)
		return strings.HasPrefix(l, "about/") || l == "loadfolders.xml" || strings.HasSuffix(l, ".xml") && (strings.Contains(l, "defs/") || strings.Contains(l, "patches/")) ||
			strings.HasPrefix(l, "assemblies/") && strings.HasSuffix(l, ".dll")
	}
	exec(`INSERT INTO environment(id,name,game_root,created_at) VALUES('env1','main','C:/Game',1)`)
	for i := 0; i < 12; i++ {
		inst := fmt.Sprintf("inst-%02d", i)
		envID := ""
		if i%4 == 3 {
			envID = "env1"
		}
		exec(`INSERT INTO instance(id,environment_id,lineage_id,source_id,rel_path,status,first_seen,last_seen,tombstone_at) VALUES(?,?,?,?,?,?,1,1,?)`,
			inst, envID, inst, "src", inst, map[bool]string{true: "tombstone", false: "live"}[i == 5], map[bool]any{true: 9, false: nil}[i == 5])
		var cur int64
		for k := 0; k < 1+i%3; k++ {
			cur = exec(`INSERT INTO snapshot(instance_id,scan_id,snapshot_hash) VALUES(?,?,?)`, inst, k+1, []byte{byte(i), byte(k)})
			for _, rel := range pool {
				if r.IntN(4) == 0 {
					continue
				}
				if r.IntN(3) == 0 {
					rel = strings.Replace(rel, "A", "a", 1)
				}
				var h any
				if semantic(rel) {
					h = []byte(fmt.Sprintf("%016d", r.IntN(1000)))
				}
				exec(`INSERT OR IGNORE INTO snapshot_file(snapshot_id,rel_path,size,mtime,hash) VALUES(?,?,?,?,?)`, cur, rel, r.IntN(5000), r.Int64(), h)
			}
		}
		exec(`UPDATE instance SET current_snapshot_id=? WHERE id=?`, cur, inst)
		if i == 11 {
			continue // no facts
		}
		// Def sets: 1.6 and 1.7 share one unless i%3==0.
		defSets := map[string]int64{}
		for _, mm := range []string{"1.6", "1.7"} {
			key := "k"
			if i%3 == 0 {
				key = "k" + mm
			}
			id, ok := defSets[key]
			if !ok {
				id = exec(`INSERT INTO fact_set(instance_id,kind,content_key) VALUES(?,?,?)`, inst, "def", key)
				defSets[key] = id
				files := []string{"Defs/ThingDefs/A.xml", "1.6/Defs/B.xml", "Defs/Ü/ß.xml", mm + "/Defs/C.xml"}
				for d := 0; d < 15; d++ {
					name := ""
					if d%4 == 0 {
						name = fmt.Sprintf("Base%d", d%8)
					}
					exec(`INSERT INTO def_fact(set_id,def_type,def_name,name,parent,abstract,file,line,may_require,may_require_any) VALUES(?,?,?,?,?,?,?,?,?,?)`,
						id, "ThingDef", fmt.Sprintf("Def%d", (d*7+i)%11), name, "P", d%2, files[r.IntN(len(files))], d*3+1, "", "x.y")
				}
			}
			var dll any
			if i%2 == 0 {
				did := exec(`INSERT INTO fact_set(instance_id,kind,content_key) VALUES(?,?,?)`, inst, "dll", "d"+mm)
				dll = did
				for _, f := range []string{"Assemblies/Mod.dll", "1.6/Assemblies/Other.dll"} {
					exec(`INSERT INTO assembly_fact(set_id,file,types,patches) VALUES(?,?,2,1)`, did, f)
					exec(`INSERT INTO type_fact(set_id,file,type_name) VALUES(?,?,?)`, did, f, "Ns.Outer+Inner")
					exec(`INSERT INTO type_fact(set_id,file,type_name) VALUES(?,?,?)`, did, f, fmt.Sprintf("Ns.T%d", i%3))
					exec(`INSERT INTO harmony_fact(set_id,file,patch_type,patch_method,kind,target,skips,dynamic) VALUES(?,?,?,?,?,?,?,?)`, did, f, "Ns.Patch", "Prefix", "prefix", fmt.Sprintf("Verse.Pawn.Tick%d()", i%2), i%2, i%4)
					exec(`INSERT INTO harmony_id(set_id,file,id) VALUES(?,?,?)`, did, f, "Ns.Harmony")
				}
			}
			exec(`INSERT INTO content_state(instance_id,game_mm,content_key,def_set_id,dll_set_id) VALUES(?,?,?,?,?)`, inst, mm, key, id, dll)
		}
	}
}

// v27Reads is what the v27 queries return, by name.
func v27Reads(t *testing.T, db *sql.DB) map[string]any {
	t.Helper()
	out := map[string]any{}
	lines := func(q string, args ...any) []string {
		t.Helper()
		rows, err := db.Query(q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		var ls []string
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			rows.Scan(ptrs...)
			ls = append(ls, j(vals...))
		}
		return ls
	}
	snaps := lines(`SELECT id FROM snapshot ORDER BY id`)
	for _, id := range snaps {
		m := map[string]string{}
		for _, l := range lines(`SELECT rel_path, size, hash IS NOT NULL FROM snapshot_file WHERE snapshot_id=?`, id) {
			rel, _, _ := strings.Cut(l, "|")
			m[strings.ToLower(rel)] = l
		}
		if len(m) == 0 {
			m = nil
		}
		out["files "+id] = m
	}
	for _, like := range v28Likes {
		out["like "+like] = lines(`SELECT i.id, f.rel_path FROM instance i JOIN snapshot_file f ON f.snapshot_id = i.current_snapshot_id
			WHERE i.environment_id IN ('', ?) AND i.tombstone_at IS NULL AND f.rel_path LIKE ? ORDER BY i.id, f.rel_path`, "env1", like)
	}
	const defs = `c.instance_id, f.def_type, f.def_name, f.name, f.parent, f.abstract, f.file, f.line, f.may_require, f.may_require_any`
	for _, mm := range []string{"1.6", "1.7"} {
		for i := 0; i < 12; i++ {
			inst := fmt.Sprintf("inst-%02d", i)
			out[mm+" defsof "+inst] = lines(`SELECT `+defs+` FROM def_fact f JOIN content_state c ON c.def_set_id=f.set_id WHERE c.instance_id=? AND c.game_mm=? ORDER BY f.rowid`, inst, mm)
			out[mm+" asm "+inst] = lines(`SELECT c.instance_id, f.file, f.skipped, f.manual, f.uses_harmony, f.types, f.patches, f.error FROM assembly_fact f JOIN content_state c ON c.dll_set_id=f.set_id WHERE c.instance_id=? AND c.game_mm=? ORDER BY f.rowid`, inst, mm)
			out[mm+" cached "+inst] = lines(`SELECT ?, f.def_type, f.def_name, f.name, f.parent, f.abstract, f.file, f.line, f.may_require, f.may_require_any FROM def_fact f
				WHERE f.set_id=(SELECT def_set_id FROM content_state WHERE instance_id=? AND game_mm=?) ORDER BY f.rowid`, inst, inst, mm)
		}
		for d := 0; d < 11; d++ {
			out[mm+" defname"+fmt.Sprint(d)] = lines(`SELECT `+defs+` FROM def_fact f CROSS JOIN content_state c ON c.def_set_id=f.set_id WHERE f.def_name=? AND c.game_mm=? ORDER BY c.instance_id, f.rowid`, fmt.Sprintf("Def%d", d), mm)
		}
		for d := 0; d < 8; d++ {
			out[mm+" name"+fmt.Sprint(d)] = lines(`SELECT `+defs+` FROM def_fact f CROSS JOIN content_state c ON c.def_set_id=f.set_id WHERE f.name=? AND f.name<>'' AND c.game_mm=? ORDER BY c.instance_id, f.rowid`, fmt.Sprintf("Base%d", d), mm)
		}
		out[mm+" harmony"] = lines(`SELECT c.instance_id, f.file, f.patch_type, f.patch_method, f.kind, f.target, f.skips, f.dynamic FROM harmony_fact f JOIN content_state c ON c.dll_set_id=f.set_id WHERE c.game_mm=? ORDER BY target, f.rowid`, mm)
		for _, ty := range []string{"Ns.Outer+Inner", "Ns.T0", "Ns.T2"} {
			ls := lines(`SELECT DISTINCT c.instance_id, f.file FROM type_fact f CROSS JOIN content_state c ON c.dll_set_id=f.set_id WHERE f.type_name=? AND c.game_mm=?`, ty, mm)
			slices.Sort(ls)
			out[mm+" types "+ty] = ls
		}
	}
	return out
}

var v28Likes = []string{"%Textures/%", "%.dll", "%mod.dll", "%x_y.dll", "%.png", "%/a\\_%", "Languages/_%/Keyed/%", "%中文%", "%100%%", "%", "%ü%", "textures/%"}

// v28Reads is the same through the Store's methods.
func v28Reads(t *testing.T, s *Store) map[string]any {
	t.Helper()
	out := map[string]any{}
	var snaps []int64
	rows, _ := s.DB.Query(`SELECT id FROM snapshot ORDER BY id`)
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		snaps = append(snaps, id)
	}
	rows.Close()
	for _, id := range snaps {
		fs, err := s.SnapshotFiles(id)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]string
		for k, f := range fs {
			if m == nil {
				m = map[string]string{}
			}
			m[k] = j(f.Rel, f.Size, f.Hash != nil)
		}
		out[fmt.Sprint("files ", id)] = m
	}
	for _, like := range v28Likes {
		m, err := s.InstanceFiles("env1", like)
		if err != nil {
			t.Fatal(err)
		}
		var ls []string
		for id, rels := range m {
			for _, rel := range rels {
				ls = append(ls, j(id, rel))
			}
		}
		slices.Sort(ls) // by instance, then (each instance's list being in path order) by path
		out["like "+like] = ls
	}
	def := func(fs []analyze.DefFact, err error) []string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		var ls []string
		for _, f := range fs {
			ls = append(ls, j(f.Instance, f.Type, f.DefName, f.Name, f.Parent, f.Abstract, f.File, f.Line, f.MayRequire, f.MayRequireAny))
		}
		return ls
	}
	for _, mm := range []string{"1.6", "1.7"} {
		s.MM = mm
		for i := 0; i < 12; i++ {
			inst := fmt.Sprintf("inst-%02d", i)
			out[mm+" defsof "+inst] = def(s.DefsOf(inst))
			asm, err := s.Assemblies(inst)
			if err != nil {
				t.Fatal(err)
			}
			var ls []string
			for _, a := range asm {
				ls = append(ls, j(a.Instance, a.File, a.Skipped, a.Manual, a.UsesHm, a.Types, a.Patches, a.Err))
			}
			out[mm+" asm "+inst] = ls
			cached, ok := s.CachedDefs(inst, s.ContentKey(inst))
			if !ok && s.ContentKey(inst) != "" {
				t.Fatalf("%s %s: cached defs missed", mm, inst)
			}
			out[mm+" cached "+inst] = def(cached, nil)
		}
		for d := 0; d < 11; d++ {
			out[mm+" defname"+fmt.Sprint(d)] = def(s.DefsNamed(fmt.Sprintf("Def%d", d), false))
		}
		for d := 0; d < 8; d++ {
			out[mm+" name"+fmt.Sprint(d)] = def(s.DefsNamed(fmt.Sprintf("Base%d", d), true))
		}
		hs, err := s.HarmonyPatches(HarmonyFilter{})
		if err != nil {
			t.Fatal(err)
		}
		var ls []string
		for _, h := range hs {
			dy := 0
			if h.Dynamic {
				dy |= 1
			}
			if h.Manual {
				dy |= 2
			}
			ls = append(ls, j(h.Instance, h.File, h.PatchType, h.PatchMethod, h.Kind, h.Target, h.Skips, dy))
		}
		out[mm+" harmony"] = ls
		for _, ty := range []string{"Ns.Outer+Inner", "Ns.T0", "Ns.T2"} {
			ps, err := s.TypeOwners0(ty)
			if err != nil {
				t.Fatal(err)
			}
			var ls []string
			for _, p := range ps {
				ls = append(ls, j(p[0], p[1]))
			}
			slices.Sort(ls)
			out[mm+" types "+ty] = ls
		}
	}
	return out
}

// 02 §7, 03 §3 (v28): the upgrade moves files into blocks and fact paths into set lists, and
// every reader returns what the v27 queries returned.
func TestMigrateV28(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archo.db")
	db := v27Store(t, path)
	v27Fill(t, db)
	before := v27Reads(t, db)
	var listed int64
	db.QueryRow(`SELECT count(*) FROM snapshot_file WHERE hash IS NULL AND rel_path NOT LIKE '%.dll'`).Scan(&listed)
	db.Close()
	var reports []UpgradeProgress
	s, err := OpenUpgrading(path, func(p UpgradeProgress) { reports = append(reports, p) })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	after := v28Reads(t, s)
	filled := 0
	for k, want := range before {
		got := after[k]
		if reflect.ValueOf(want).Len() > 0 {
			filled++
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s:\n got %v\nwant %v", k, got, want)
		}
	}
	if filled < len(before)*3/4 {
		t.Fatalf("only %d of %d reads return anything", filled, len(before))
	}
	if q1(t, s, `SELECT value FROM meta WHERE key='schema_version'`) != fmt.Sprint(SchemaVersion) {
		t.Fatal("version")
	}
	if got := q1(t, s, `SELECT coalesce(sum(n),0) FROM snapshot_list`); got != fmt.Sprint(listed) || listed == 0 {
		t.Fatalf("%s files in blocks, want %d", got, listed)
	}
	if got := q1(t, s, `SELECT count(*) FROM snapshot_file WHERE hash IS NULL AND rel_path NOT LIKE '%.dll'`); got != "0" {
		t.Fatal("non-semantic rows left", got)
	}
	if got := q1(t, s, `SELECT count(*) FROM pragma_foreign_key_check`); got != "0" {
		t.Fatal("foreign keys", got)
	}
	if got := q1(t, s, `SELECT count(*) FROM sqlite_master WHERE name LIKE '%_v27' OR name LIKE 'set_file_v28'`); got != "0" {
		t.Fatal("old tables left", got)
	}
	if len(reports) < 2 || reports[0].From != 27 || reports[0].To != SchemaVersion {
		t.Fatalf("progress %v", reports)
	}
	last := reports[len(reports)-1]
	if last.Total == 0 || last.Done != last.Total {
		t.Fatalf("progress ends at %d of %d", last.Done, last.Total)
	}
	for i := 1; i < len(reports); i++ {
		if reports[i].Done < reports[i-1].Done {
			t.Fatalf("progress goes back: %v", reports)
		}
	}
	// A current database reports nothing.
	s.Close()
	reports = nil
	s, err = OpenUpgrading(path, func(p UpgradeProgress) { reports = append(reports, p) })
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if len(reports) != 0 {
		t.Fatal(reports)
	}
}

// j joins values with "|", booleans as 0 and 1 (as SQLite returns them).
func j(vals ...any) string {
	ss := make([]string, len(vals))
	for i, v := range vals {
		switch x := v.(type) {
		case bool:
			ss[i] = map[bool]string{true: "1", false: "0"}[x]
		case []byte:
			ss[i] = string(x)
		default:
			ss[i] = fmt.Sprint(x)
		}
	}
	return strings.Join(ss, "|")
}

func TestFileListCodec(t *testing.T) {
	es := []listEntry{{"", 0}, {"A", 1}, {"Textures/中文/图.png", 1 << 40}, {"Textures/中文/图2.png", 3}, {"z", 0}}
	data, err := encodeList(es)
	if err != nil {
		t.Fatal(err)
	}
	var got []listEntry
	if err := decodeList(data, func(rel string, size int64) { got = append(got, listEntry{rel, size}) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, es) {
		t.Fatalf("%v", got)
	}
	if err := decodeList(data[:len(data)/2], func(string, int64) {}); err == nil {
		t.Fatal("a cut block decoded")
	}
}

// likeMatch is SQLite's LIKE: % and _, ASCII letters without case, other letters exact.
func TestLikeMatchesSQLite(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subjects := []string{"", "a", "A", "Textures/a.png", "textures/A.PNG", "x/Textures/b/c.png", "Ü.png", "ü.png", "中文/图.png", "a_b.dll", "aXb.dll",
		"100%.txt", "Languages/English/Keyed/A.xml", "Languages//Keyed/x", "abcabc", "ab", "mod.DLL"}
	patterns := []string{"", "%", "_", "a", "%Textures/%", "%.png", "%a_b%", "a%b%c", "%abc", "abc%", "%Ü%", "%ü%", "_.png", "__", "%中文%", "%100%%",
		"Languages/_%/Keyed/%", "%%", "%_%", "%.dll", "%mod.dll", "a%"}
	for _, p := range patterns {
		for _, s := range subjects {
			var want bool
			if err := db.QueryRow(`SELECT ? LIKE ?`, s, p).Scan(&want); err != nil {
				t.Fatal(err)
			}
			if got := likeMatch(p, s); got != want {
				t.Errorf("%q LIKE %q = %v, SQLite says %v", s, p, got, want)
			}
		}
	}
}
