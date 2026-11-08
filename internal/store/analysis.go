package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/finding"
)

// ContentKey returns an instance's stored effective-content key ("" if none).
func (s *Store) ContentKey(instanceID string) string {
	var k string
	s.DB.QueryRow(`SELECT content_key FROM content_state WHERE instance_id=? AND game_mm=?`, instanceID, s.MM).Scan(&k)
	return k
}

// ContentKeys returns every instance's stored effective-content key for s.MM.
func (s *Store) ContentKeys() (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT instance_id, content_key FROM content_state WHERE game_mm=?`, s.MM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, err
		}
		out[id] = key
	}
	return out, rows.Err()
}

// CachedDefs returns an instance's Def facts if they were built for key.
func (s *Store) CachedDefs(instanceID, key string) ([]analyze.DefFact, bool) {
	// The analysis reads every inactive mod's set once: its list comes with the set, not
	// through the cache (setfiles.go).
	var id int64
	var data string
	if err := s.DB.QueryRow(`SELECT id, files FROM fact_set WHERE instance_id=? AND kind='def' AND content_key=?`, instanceID, key).Scan(&id, &data); err != nil {
		return nil, false
	}
	files, err := decodeFiles(id, data)
	if err != nil {
		return nil, false
	}
	rows, err := s.DB.Query(`SELECT ? AS instance_id,`+defFields+` FROM def_fact f WHERE f.set_id=? ORDER BY f.rowid`, instanceID, id)
	if err != nil {
		return nil, false
	}
	fs, err := s.scanDefsWith(rows, func(set int64) []string { return files })
	return fs, err == nil
}

// defFields: file_ix indexes the set's file list (05 §3); scanDefs turns it into the path.
const defFields = `f.set_id, f.def_type, f.def_name, f.name, f.parent, f.abstract, f.file_ix, f.line, f.may_require, f.may_require_any`
const defCols = `c.instance_id, ` + defFields
const defView = ` FROM def_fact f JOIN content_state c ON c.def_set_id=f.set_id `

// defByName is defView for lookups by name: CROSS JOIN keeps the name index as the outer
// loop. With a plain JOIN, ORDER BY c.instance_id makes SQLite walk every content_state row
// of the version and probe the name index once per row: tens of times slower, more with
// more mods (18 §2).
const defByName = ` FROM def_fact f CROSS JOIN content_state c ON c.def_set_id=f.set_id `

// The DefsNamed queries, by defName and by Name attribute.
const (
	defsByDefNameQ = `SELECT ` + defCols + defByName + `WHERE f.def_name=? AND c.game_mm=? ORDER BY c.instance_id, f.rowid`
	defsByNameQ    = `SELECT ` + defCols + defByName + `WHERE f.name=? AND f.name<>'' AND c.game_mm=? ORDER BY c.instance_id, f.rowid`
)

func (s *Store) scanDefs(rows *sql.Rows) ([]analyze.DefFact, error) { return s.scanDefsWith(rows, nil) }

// scanDefsWith: with lists, the file lists are known (CachedDefs).
func (s *Store) scanDefsWith(rows *sql.Rows, lists func(set int64) []string) ([]analyze.DefFact, error) {
	defer rows.Close()
	var out []analyze.DefFact
	var at []fileRef
	for rows.Next() {
		var f analyze.DefFact
		var abs int
		var r fileRef
		if err := rows.Scan(&f.Instance, &r.set, &f.Type, &f.DefName, &f.Name, &f.Parent, &abs, &r.ix, &f.Line, &f.MayRequire, &f.MayRequireAny); err != nil {
			return nil, err
		}
		f.Abstract = abs != 0
		out = append(out, f)
		at = append(at, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if lists != nil {
		for i, r := range at {
			l := lists(r.set)
			if r.ix < 0 || r.ix >= len(l) {
				return nil, fmt.Errorf("store: fact set %d has no file %d", r.set, r.ix)
			}
			out[i].File = l[r.ix]
		}
		return out, nil
	}
	for i := range at {
		at[i].to = &out[i].File
	}
	return out, s.resolveFiles(at)
}

// DefsOf returns the Def facts of one instance, in file order.
func (s *Store) DefsOf(instanceID string) ([]analyze.DefFact, error) {
	rows, err := s.DB.Query(`SELECT `+defCols+defView+` WHERE c.instance_id=? AND c.game_mm=? ORDER BY f.rowid`, instanceID, s.MM)
	if err != nil {
		return nil, err
	}
	return s.scanDefs(rows)
}

// DefsNamed returns the Def facts with a defName (or, with byName, a Name attribute), by
// instance ID and then file order. The order must not follow the table layout: rebuilt or
// migrated facts get new rowids, and the first provider of a missing parent (and so a log
// problem's ID) would change with them (05 §3).
func (s *Store) DefsNamed(name string, byName bool) ([]analyze.DefFact, error) {
	q := defsByDefNameQ
	if byName {
		q = defsByNameQ
	}
	rows, err := s.DB.Query(q, name, s.MM)
	if err != nil {
		return nil, err
	}
	return s.scanDefs(rows)
}

// DefCounts returns the number of Def facts per instance.
func (s *Store) DefCounts() (map[string]int, error) {
	rows, err := s.DB.Query(`SELECT c.instance_id, count(*)`+defView+` WHERE c.game_mm=? GROUP BY c.instance_id`, s.MM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ContentFolders returns an instance's effective load folders (highest priority first).
func (s *Store) ContentFolders(instanceID string) []string {
	var f string
	s.DB.QueryRow(`SELECT folders FROM content_state WHERE instance_id=? AND game_mm=?`, instanceID, s.MM).Scan(&f)
	if f == "" {
		return nil
	}
	return strings.Split(f, "\n")
}

// Analysis is one stored analysis run.
type Analysis struct {
	ID       int64
	SetKey   string // the enabled set's fingerprint: the run's slot (05 §3)
	RunKey   string
	MM       string
	Version  int
	Started  time.Time
	Finished time.Time
	Stats    analyze.Stats
}

// SaveAnalysis persists a run: content states and rebuilt Def facts, the Patch facts and
// findings. The run fills the slot (environment, mm, setKey) (05 §3): the slot's older run
// is deleted, and so is every run of the environment whose set key is not in keep
// (the enabled sets the environment's plans have now), so slots do not pile up as plans
// are edited.
func (s *Store) SaveAnalysis(envID, setKey, runKey, mm string, keep []string, out *analyze.Output, started, finished time.Time) (int64, error) {
	var id int64
	err := s.inTx(func(tx *sql.Tx) error {
		if err := saveContent(tx, out, mm); err != nil {
			return err
		}
		kept := map[string]bool{}
		for _, k := range keep {
			kept[k] = true
		}
		var old []int64
		rows, err := tx.Query(`SELECT id, set_key, game_mm FROM analysis WHERE environment_id=?`, envID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var x int64
			var k, oldMM string
			if err := rows.Scan(&x, &k, &oldMM); err != nil {
				rows.Close()
				return err
			}
			if oldMM == mm && k == setKey || !kept[k] {
				old = append(old, x)
			}
		}
		rows.Close()
		for _, x := range old {
			for _, q := range []string{`DELETE FROM patch_fact WHERE analysis_id=?`, `DELETE FROM finding WHERE analysis_id=?`, `DELETE FROM analysis WHERE id=?`} {
				if _, err := tx.Exec(q, x); err != nil {
					return err
				}
			}
		}
		st, _ := json.Marshal(out.Stats)
		res, err := tx.Exec(`INSERT INTO analysis(environment_id, set_key, run_key, game_mm, version, started_at, finished_at, stats) VALUES(?,?,?,?,?,?,?,?)`,
			envID, setKey, runKey, mm, analyze.Version, started.Unix(), finished.Unix(), string(st))
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		pins, err := tx.Prepare(`INSERT INTO patch_fact(analysis_id, instance_id, seq, file, line, class, xpath, descr, result, hits, reasons, detail, targets, leaf)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer pins.Close()
		for _, p := range out.Patches {
			if _, err := pins.Exec(id, p.Instance, p.Seq, p.File, p.Line, p.Class, p.XPath, p.Desc, p.Result, p.Hits,
				strings.Join(p.Reasons, " "), p.Detail, strings.Join(p.Targets, " "), p.Leaf); err != nil {
				return err
			}
		}
		fins, err := tx.Prepare(`INSERT INTO finding(analysis_id, layer, code, severity, key, subject_instance, data) VALUES(?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer fins.Close()
		for _, f := range out.Findings {
			b, _ := json.Marshal(f)
			if _, err := fins.Exec(id, f.Layer, f.Code, f.Severity, f.Key, f.SubjectInstance(), string(b)); err != nil {
				return err
			}
		}
		return pruneFacts(tx, finished)
	})
	return id, err
}

// LatestAnalysis returns the stored run of the slot (environment, Store.MM, setKey) (nil if
// none).
func (s *Store) LatestAnalysis(envID, setKey string) (*Analysis, error) {
	return s.latestAnalysis(`SELECT id, set_key, run_key, game_mm, version, started_at, finished_at, stats FROM analysis
		WHERE environment_id=? AND game_mm=? AND set_key=? ORDER BY id DESC LIMIT 1`, envID, s.MM, setKey)
}

// NewestAnalysis returns the environment's newest run for Store.MM in any slot (nil if
// none).
func (s *Store) NewestAnalysis(envID string) (*Analysis, error) {
	return s.latestAnalysis(`SELECT id, set_key, run_key, game_mm, version, started_at, finished_at, stats FROM analysis
		WHERE environment_id=? AND game_mm=? ORDER BY id DESC LIMIT 1`, envID, s.MM)
}

func (s *Store) latestAnalysis(q string, args ...any) (*Analysis, error) {
	var a Analysis
	var st string
	var started, finished int64
	err := s.DB.QueryRow(q, args...).
		Scan(&a.ID, &a.SetKey, &a.RunKey, &a.MM, &a.Version, &started, &finished, &st)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.Started, a.Finished = time.Unix(started, 0).UTC(), time.Unix(finished, 0).UTC()
	json.Unmarshal([]byte(st), &a.Stats)
	return &a, nil
}

// Findings returns the stored Def/Patch-layer findings of a run.
func (s *Store) Findings(analysisID int64) ([]*finding.Finding, error) {
	rows, err := s.DB.Query(`SELECT data FROM finding WHERE analysis_id=? ORDER BY rowid`, analysisID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*finding.Finding
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		var f finding.Finding
		json.Unmarshal([]byte(d), &f)
		out = append(out, &f)
	}
	return out, rows.Err()
}

// PatchFilter selects Patch facts.
type PatchFilter struct {
	Instance   string
	FailedOnly bool
	Target     string // "ThingDef/Gun_A": facts whose touched defs or XPath mention it
	XPathPart  string // substring of the XPath
	// DefName widens XPathPart: also facts whose touched defs (Targets) include a def of
	// that name, and failed facts whose innermost XPath has XPathPart. A FindMod or
	// Sequence has no XPath of its own; its nested operations are found this way (P26).
	DefName string
}

// likeEscape escapes LIKE wildcards for ESCAPE '\'.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}

// PatchFacts returns the Patch facts of a run, in load order of their files.
func (s *Store) PatchFacts(analysisID int64, f PatchFilter) ([]*analyze.PatchFact, error) {
	q := `SELECT instance_id, seq, file, line, class, xpath, descr, result, hits, reasons, detail, targets, leaf FROM patch_fact WHERE analysis_id=?`
	args := []any{analysisID}
	if f.Instance != "" {
		q += ` AND instance_id=?`
		args = append(args, f.Instance)
	}
	if f.FailedOnly {
		q += ` AND result='failed'`
	}
	if f.XPathPart != "" && f.DefName != "" {
		q += ` AND (instr(xpath, ?) > 0 OR instr(leaf, ?) > 0 OR ' ' || targets || ' ' LIKE ? ESCAPE '\')`
		args = append(args, f.XPathPart, f.XPathPart, "%/"+likeEscape(f.DefName)+" %")
	} else if f.XPathPart != "" {
		q += ` AND instr(xpath, ?) > 0`
		args = append(args, f.XPathPart)
	}
	if f.Target != "" {
		// "_" in a def name is a LIKE wildcard: Lib_Gun must not match LibXGun.
		q += ` AND (' ' || targets || ' ' LIKE ? ESCAPE '\' OR instr(leaf, ?) > 0)`
		name := f.Target
		if i := strings.IndexAny(name, "/@"); i >= 0 {
			name = name[i+1:]
		}
		args = append(args, "% "+likeEscape(f.Target)+" %", `"`+name+`"`)
	}
	rows, err := s.DB.Query(q+` ORDER BY rowid`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*analyze.PatchFact
	for rows.Next() {
		var p analyze.PatchFact
		var reasons, targets string
		if err := rows.Scan(&p.Instance, &p.Seq, &p.File, &p.Line, &p.Class, &p.XPath, &p.Desc, &p.Result, &p.Hits, &reasons, &p.Detail, &targets, &p.Leaf); err != nil {
			return nil, err
		}
		p.Reasons, p.Targets = strings.Fields(reasons), strings.Fields(targets)
		out = append(out, &p)
	}
	return out, rows.Err()
}

// PatchCounts returns (operations, failed) per instance for a run.
func (s *Store) PatchCounts(analysisID int64) (map[string][2]int, error) {
	rows, err := s.DB.Query(`SELECT instance_id, count(*), sum(result='failed') FROM patch_fact WHERE analysis_id=? GROUP BY instance_id`, analysisID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][2]int{}
	for rows.Next() {
		var id string
		var n, f int
		if err := rows.Scan(&id, &n, &f); err != nil {
			return nil, err
		}
		out[id] = [2]int{n, f}
	}
	return out, rows.Err()
}

// Ignore is a problem the player chose to ignore (05 §4.2).
type Ignore struct {
	ID         int64
	LineageID  string
	Code, Key  string
	SnapshotID int64
	CreatedAt  time.Time
}

// Ignores returns the environment's ignore records.
func (s *Store) Ignores(envID string) ([]*Ignore, error) {
	rows, err := s.DB.Query(`SELECT id, lineage_id, code, key, snapshot_id, created_at FROM finding_ignore WHERE environment_id=? ORDER BY id`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Ignore
	for rows.Next() {
		var g Ignore
		var at int64
		if err := rows.Scan(&g.ID, &g.LineageID, &g.Code, &g.Key, &g.SnapshotID, &at); err != nil {
			return nil, err
		}
		g.CreatedAt = time.Unix(at, 0).UTC()
		out = append(out, &g)
	}
	return out, rows.Err()
}

// SetIgnore adds or refreshes an ignore record.
func (s *Store) SetIgnore(envID string, g *Ignore) error {
	_, err := s.DB.Exec(`INSERT INTO finding_ignore(environment_id, lineage_id, code, key, snapshot_id, created_at) VALUES(?,?,?,?,?,?)
		ON CONFLICT(environment_id, lineage_id, code, key) DO UPDATE SET snapshot_id=excluded.snapshot_id, created_at=excluded.created_at`,
		envID, g.LineageID, g.Code, g.Key, g.SnapshotID, g.CreatedAt.Unix())
	return err
}

// RemoveIgnore deletes an ignore record; it reports whether one existed.
func (s *Store) RemoveIgnore(envID, lineageID, code, key string) (bool, error) {
	r, err := s.DB.Exec(`DELETE FROM finding_ignore WHERE environment_id=? AND lineage_id=? AND code=? AND key=?`, envID, lineageID, code, key)
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n > 0, nil
}

// DefCount is the number of Def facts of an instance.
func (s *Store) DefCount(instanceID string) int {
	var n int
	s.DB.QueryRow(`SELECT count(*)`+defView+` WHERE c.instance_id=? AND c.game_mm=?`, instanceID, s.MM).Scan(&n)
	return n
}

// PatchCount returns an instance's operations and failures in a run.
func (s *Store) PatchCount(analysisID int64, instanceID string) (ops, failed int) {
	s.DB.QueryRow(`SELECT count(*), coalesce(sum(result='failed'), 0) FROM patch_fact WHERE analysis_id=? AND instance_id=?`, analysisID, instanceID).Scan(&ops, &failed)
	return
}

func saveHarmony(tx *sql.Tx, asms []*analyze.AssemblyFacts, sets map[string]int64) error {
	insA, err := tx.Prepare(`INSERT INTO assembly_fact(set_id, file_ix, skipped, manual, uses_harmony, types, patches, error) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insA.Close()
	insT, err := tx.Prepare(`INSERT INTO type_fact(set_id, file_ix, type_name) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer insT.Close()
	insH, err := tx.Prepare(`INSERT INTO harmony_fact(set_id, file_ix, patch_type, patch_method, kind, target, target_type, target_method, args, skips, dynamic) VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insH.Close()
	insI, err := tx.Prepare(`INSERT INTO harmony_id(set_id, file_ix, id) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer insI.Close()
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	lists := map[int64]*fileIndex{}
	var order []int64
	for _, a := range asms {
		set := sets[a.Instance]
		if set == 0 {
			continue
		}
		x := lists[set]
		if x == nil {
			x = &fileIndex{}
			lists[set] = x
			order = append(order, set)
		}
		file := x.of(a.File)
		if _, err := insA.Exec(set, file, b(a.Skipped), b(a.Manual), b(a.UsesHarm), len(a.Types), len(a.Patches), a.Err); err != nil {
			return err
		}
		for _, t := range a.Types {
			if _, err := insT.Exec(set, file, t); err != nil {
				return err
			}
		}
		for _, p := range a.Patches {
			// dynamic holds two bits: 1 the target is chosen at run time, 2 the patch was
			// found at a Harmony.Patch call in code (G9, 05 §5.3).
			if _, err := insH.Exec(set, file, p.PatchType, p.PatchMethod, p.Kind, p.Target(), p.TargetType, p.TargetMethod, p.Args, b(p.Skips), b(p.Dynamic)|2*b(p.Manual)); err != nil {
				return err
			}
		}
		for _, id := range a.IDs {
			if _, err := insI.Exec(set, file, id); err != nil {
				return err
			}
		}
	}
	for _, set := range order {
		if _, err := tx.Exec(`UPDATE fact_set SET files=? WHERE id=?`, lists[set].json(), set); err != nil {
			return err
		}
	}
	return nil
}

// HarmonyRow is a stored Harmony patch.
type HarmonyRow struct {
	Instance, File, PatchType, PatchMethod, Kind, Target string
	Skips, Dynamic                                       bool
	// Manual: found at a Harmony.Patch call in code, not from attributes (G9). With
	// Dynamic its original was not resolved and Target says why ("<manual: …>").
	Manual bool
}

// HarmonyFilter selects Harmony patches.
type HarmonyFilter struct {
	Instance string
	Target   string // exact target (any overload), or with Like a case-insensitive substring
	Like     bool
}

// HarmonyPatches returns stored patches ordered by target.
func (s *Store) HarmonyPatches(f HarmonyFilter) ([]*HarmonyRow, error) {
	q := `SELECT c.instance_id, f.set_id, f.file_ix, f.patch_type, f.patch_method, f.kind, f.target, f.skips, f.dynamic FROM harmony_fact f JOIN content_state c ON c.dll_set_id=f.set_id WHERE c.game_mm=?`
	args := []any{s.MM}
	if f.Instance != "" {
		q += ` AND c.instance_id=?`
		args = append(args, f.Instance)
	}
	if f.Target != "" {
		if f.Like {
			q += ` AND instr(lower(target), lower(?)) > 0`
			args = append(args, f.Target)
		} else {
			q += ` AND (target=? OR substr(target, 1, length(?)+1)=? || '(')`
			args = append(args, f.Target, f.Target, f.Target)
		}
	}
	rows, err := s.DB.Query(q+` ORDER BY target, f.rowid`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*HarmonyRow
	var at []fileRef
	for rows.Next() {
		h := &HarmonyRow{}
		var sk, dy int
		r := fileRef{to: &h.File}
		if err := rows.Scan(&h.Instance, &r.set, &r.ix, &h.PatchType, &h.PatchMethod, &h.Kind, &h.Target, &sk, &dy); err != nil {
			return nil, err
		}
		h.Skips, h.Dynamic, h.Manual = sk != 0, dy&1 != 0, dy&2 != 0
		out = append(out, h)
		at = append(at, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return out, s.resolveFiles(at)
}

// AssemblyRow is a stored assembly.
type AssemblyRow struct {
	Instance, File          string
	Skipped, Manual, UsesHm bool
	Types, Patches          int
	Err                     string
}

// Assemblies returns the stored assemblies of an instance.
func (s *Store) Assemblies(instanceID string) ([]*AssemblyRow, error) {
	rows, err := s.DB.Query(`SELECT c.instance_id, f.set_id, f.file_ix, f.skipped, f.manual, f.uses_harmony, f.types, f.patches, f.error FROM assembly_fact f JOIN content_state c ON c.dll_set_id=f.set_id WHERE c.instance_id=? AND c.game_mm=? ORDER BY f.rowid`, instanceID, s.MM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AssemblyRow
	var at []fileRef
	for rows.Next() {
		a := &AssemblyRow{}
		var sk, ma, uh int
		r := fileRef{to: &a.File}
		if err := rows.Scan(&a.Instance, &r.set, &r.ix, &sk, &ma, &uh, &a.Types, &a.Patches, &a.Err); err != nil {
			return nil, err
		}
		a.Skipped, a.Manual, a.UsesHm = sk != 0, ma != 0, uh != 0
		out = append(out, a)
		at = append(at, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return out, s.resolveFiles(at)
}

// TypeOwners returns the (instance, file) pairs defining a type full name. Nested types are
// stored with '+'; stack frames may print either.
func (s *Store) TypeOwners(name string) ([][2]string, error) {
	var out [][2]string
	err := nestedForms(name, func(names ...string) (bool, error) {
		var err error
		out, err = s.typeOwners(`f.type_name IN (`+placeholders(len(names))+`)`, anys(names)...)
		return len(out) > 0, err
	})
	return out, err
}

// nestedForms calls look with the stored forms of a type name until one finds something:
// the name as given together with every '.' turned into '+', then (a nested type printed
// with dots) the last dots turned into '+' one at a time: "A.B.Outer.Inner" →
// "A.B.Outer+Inner", then "A.B+Outer+Inner".
func nestedForms(name string, look func(names ...string) (bool, error)) error {
	if ok, err := look(name, strings.ReplaceAll(name, ".", "+")); ok || err != nil {
		return err
	}
	parts := strings.Split(name, ".")
	for k := len(parts) - 1; k > 1; k-- {
		cand := strings.Join(parts[:k], ".") + "+" + strings.Join(parts[k:], "+")
		if ok, err := look(cand); ok || err != nil {
			return err
		}
	}
	return nil
}

func anys(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// TypeOwners0 is TypeOwners for an exact stored name.
func (s *Store) TypeOwners0(name string) ([][2]string, error) {
	return s.typeOwners(`f.type_name=?`, name)
}

// typeOwners lists the distinct (instance, file) pairs of the matching type facts, in the
// order the rows come.
func (s *Store) typeOwners(where string, args ...any) ([][2]string, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT c.instance_id, f.set_id, f.file_ix FROM type_fact f CROSS JOIN content_state c ON c.dll_set_id=f.set_id WHERE `+where+` AND c.game_mm=?`, append(args, s.MM)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	var at []fileRef
	for rows.Next() {
		var a string
		var r fileRef
		if err := rows.Scan(&a, &r.set, &r.ix); err != nil {
			return nil, err
		}
		out = append(out, [2]string{a})
		at = append(at, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range at {
		at[i].to = &out[i][1]
	}
	// One DLL view per instance and version, and a list names each file once: the
	// distinct (instance, set, index) rows are the distinct (instance, file) pairs.
	return out, s.resolveFiles(at)
}

// HarmonyIDOwners returns the instances whose assemblies contain the Harmony ID string.
func (s *Store) HarmonyIDOwners(id string) ([]string, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT c.instance_id FROM harmony_id f CROSS JOIN content_state c ON c.dll_set_id=f.set_id WHERE f.id=? AND c.game_mm=?`, id, s.MM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		rows.Scan(&a)
		out = append(out, a)
	}
	return out, rows.Err()
}

// DecompileRecord is one source_use row.
type DecompileRecord struct {
	Hash, Instance, File, Status, Err string
	At                                time.Time
}

// RecordDecompile notes a decompile attempt.
func (s *Store) RecordDecompile(hash, instance, file, status, errText string, at time.Time) error {
	return s.RecordSourceUse(hash, instance, file, "project", status, errText, at)
}

// DecompileRecords returns the decompile history of an instance, newest first.
func (s *Store) DecompileRecords(instance string) ([]*DecompileRecord, error) {
	rows, err := s.DB.Query(`SELECT dll_hash, instance_id, file, status, error, decompiled_at FROM source_use WHERE instance_id=? ORDER BY decompiled_at DESC`, instance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DecompileRecord
	for rows.Next() {
		var r DecompileRecord
		var at int64
		if err := rows.Scan(&r.Hash, &r.Instance, &r.File, &r.Status, &r.Err, &at); err != nil {
			return nil, err
		}
		r.At = time.Unix(at, 0).UTC()
		out = append(out, &r)
	}
	return out, rows.Err()
}

// TypesUnder returns the distinct type names of Store.MM's analysed assemblies in the root
// namespace root ("VanillaFurnitureEC" → "VanillaFurnitureEC.…"), each with the instances
// defining it, at most limit names: the candidates a misspelled class is compared with
// (06 §2, "did you mean").
func (s *Store) TypesUnder(root string, limit int) (map[string][]string, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT f.type_name, c.instance_id FROM type_fact f CROSS JOIN content_state c ON c.dll_set_id=f.set_id
 WHERE f.type_name >= ? AND f.type_name < ? AND instr(f.type_name,'<')=0 AND c.game_mm=? LIMIT ?`, root+".", root+"/", s.MM, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var name, inst string
		if err := rows.Scan(&name, &inst); err != nil {
			return nil, err
		}
		out[name] = append(out[name], inst)
	}
	return out, rows.Err()
}

// TypesNamed returns up to limit type full names of Store.MM (Mods' analysed assemblies and the
// game's) whose last segment equals the last segment of name: the candidates offered when a
// type is asked for under the wrong namespace ("RimWorld.PlayLogEntry_Interaction" →
// "Verse.PlayLogEntry_Interaction").
func (s *Store) TypesNamed(name string, limit int) ([]string, error) {
	short := name
	if i := strings.LastIndexAny(short, ".+"); i >= 0 {
		short = short[i+1:]
	}
	if short == "" {
		return nil, nil
	}
	rows, err := s.DB.Query(`SELECT name FROM (
 SELECT f.type_name AS name FROM type_fact f CROSS JOIN content_state c ON c.dll_set_id=f.set_id
  WHERE (f.type_name=?1 OR f.type_name LIKE ?2 ESCAPE '\' OR f.type_name LIKE ?3 ESCAPE '\') AND c.game_mm=?4
 UNION
 SELECT g.type_name FROM game_type g JOIN game_assembly a ON a.id=g.assembly_id
  WHERE (g.type_name=?1 OR g.type_name LIKE ?2 ESCAPE '\' OR g.type_name LIKE ?3 ESCAPE '\') AND a.game_mm=?4)
 WHERE name<>?5 ORDER BY name LIMIT ?6`, short, "%."+likeEscape(short), "%+"+likeEscape(short), s.MM, name, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// GameTypesUnder is TypesUnder for the game's own assemblies (any environment of Store.MM).
func (s *Store) GameTypesUnder(root string, limit int) ([]string, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT g.type_name FROM game_type g JOIN game_assembly a ON a.id=g.assembly_id
 WHERE g.type_name >= ? AND g.type_name < ? AND a.game_mm=? LIMIT ?`, root+".", root+"/", s.MM, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// DefsInFile returns the non-abstract Def facts of Store.MM that come from a file named base
// (any folder, case-insensitive), for the instances in only: what is lost when the game
// reports "Exception loading def from file <base>" (06 §3, consequences).
func (s *Store) DefsInFile(base string, only map[string]bool) ([]analyze.DefFact, error) {
	rows, err := s.DB.Query(`SELECT c.instance_id, s.id, s.files FROM fact_set s JOIN content_state c ON c.def_set_id=s.id
 WHERE c.game_mm=? AND s.files LIKE ?`, s.MM, `%`+base+`"%`)
	if err != nil {
		return nil, err
	}
	type hit struct {
		inst string
		set  int64
		ixs  []any
	}
	var hits []hit
	for rows.Next() {
		var inst, files string
		var set int64
		if err := rows.Scan(&inst, &set, &files); err != nil {
			rows.Close()
			return nil, err
		}
		if !only[inst] {
			continue
		}
		var list []string
		if json.Unmarshal([]byte(files), &list) != nil {
			continue
		}
		h := hit{inst: inst, set: set}
		for i, f := range list {
			if strings.EqualFold(path.Base(f), base) {
				h.ixs = append(h.ixs, i)
			}
		}
		if len(h.ixs) > 0 {
			hits = append(hits, h)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var out []analyze.DefFact
	for _, h := range hits {
		r, err := s.DB.Query(`SELECT `+defCols+defView+` WHERE c.instance_id=? AND c.game_mm=? AND f.set_id=? AND f.abstract=0 AND f.def_name<>'' AND f.file_ix IN (`+placeholders(len(h.ixs))+`) ORDER BY f.rowid`,
			append([]any{h.inst, s.MM, h.set}, h.ixs...)...)
		if err != nil {
			return nil, err
		}
		ds, err := s.scanDefs(r)
		if err != nil {
			return nil, err
		}
		out = append(out, ds...)
	}
	return out, nil
}
