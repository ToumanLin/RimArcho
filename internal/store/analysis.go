package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/rimarcho/rimarcho/internal/analyze"
	"github.com/rimarcho/rimarcho/internal/finding"
)

// ContentKey returns an instance's stored effective-content key ("" if none).
func (s *Store) ContentKey(instanceID string) string {
	var k string
	s.DB.QueryRow(`SELECT content_key FROM content_state WHERE instance_id=?`, instanceID).Scan(&k)
	return k
}

// CachedDefs returns an instance's Def facts if they were built for key.
func (s *Store) CachedDefs(instanceID, key string) ([]analyze.DefFact, bool) {
	if s.ContentKey(instanceID) != key {
		return nil, false
	}
	fs, err := s.DefsOf(instanceID)
	return fs, err == nil
}

const defCols = `instance_id, def_type, def_name, name, parent, abstract, file, line, may_require, may_require_any`

func scanDefs(rows *sql.Rows) ([]analyze.DefFact, error) {
	defer rows.Close()
	var out []analyze.DefFact
	for rows.Next() {
		var f analyze.DefFact
		var abs int
		if err := rows.Scan(&f.Instance, &f.Type, &f.DefName, &f.Name, &f.Parent, &abs, &f.File, &f.Line, &f.MayRequire, &f.MayRequireAny); err != nil {
			return nil, err
		}
		f.Abstract = abs != 0
		out = append(out, f)
	}
	return out, rows.Err()
}

// DefsOf returns the Def facts of one instance, in file order.
func (s *Store) DefsOf(instanceID string) ([]analyze.DefFact, error) {
	rows, err := s.DB.Query(`SELECT `+defCols+` FROM def_fact WHERE instance_id=? ORDER BY rowid`, instanceID)
	if err != nil {
		return nil, err
	}
	return scanDefs(rows)
}

// DefsNamed returns the Def facts with a defName (or, with byName, a Name attribute).
func (s *Store) DefsNamed(name string, byName bool) ([]analyze.DefFact, error) {
	q := `SELECT ` + defCols + ` FROM def_fact WHERE def_name=? ORDER BY rowid`
	if byName {
		q = `SELECT ` + defCols + ` FROM def_fact WHERE name=? ORDER BY rowid`
	}
	rows, err := s.DB.Query(q, name)
	if err != nil {
		return nil, err
	}
	return scanDefs(rows)
}

// DefCounts returns the number of Def facts per instance.
func (s *Store) DefCounts() (map[string]int, error) {
	rows, err := s.DB.Query(`SELECT instance_id, count(*) FROM def_fact GROUP BY instance_id`)
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
	s.DB.QueryRow(`SELECT folders FROM content_state WHERE instance_id=?`, instanceID).Scan(&f)
	if f == "" {
		return nil
	}
	return strings.Split(f, "\n")
}

// Analysis is one stored analysis run.
type Analysis struct {
	ID       int64
	RunKey   string
	MM       string
	Version  int
	Started  time.Time
	Finished time.Time
	Stats    analyze.Stats
}

// SaveAnalysis persists a run: content states and rebuilt Def facts, the Patch facts and
// findings. Older runs of the environment are deleted.
func (s *Store) SaveAnalysis(envID, runKey, mm string, out *analyze.Output, started, finished time.Time) (int64, error) {
	var id int64
	err := s.inTx(func(tx *sql.Tx) error {
		for inst, cs := range out.Content {
			if _, err := tx.Exec(`INSERT INTO content_state(instance_id, content_key, folders, def_files, patch_files, parse_errors) VALUES(?,?,?,?,?,?)
				ON CONFLICT(instance_id) DO UPDATE SET content_key=excluded.content_key, folders=excluded.folders,
				def_files=excluded.def_files, patch_files=excluded.patch_files, parse_errors=excluded.parse_errors`,
				inst, cs.Key, strings.Join(cs.Folders, "\n"), cs.DefFiles, cs.PatchFiles, strings.Join(cs.ParseErrors, "\n")); err != nil {
				return err
			}
		}
		del, err := tx.Prepare(`DELETE FROM def_fact WHERE instance_id=?`)
		if err != nil {
			return err
		}
		defer del.Close()
		ins, err := tx.Prepare(`INSERT INTO def_fact(` + defCols + `) VALUES(?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer ins.Close()
		for inst, fs := range out.NewDefs {
			if _, err := del.Exec(inst); err != nil {
				return err
			}
			for _, f := range fs {
				abs := 0
				if f.Abstract {
					abs = 1
				}
				if _, err := ins.Exec(inst, f.Type, f.DefName, f.Name, f.Parent, abs, f.File, f.Line, f.MayRequire, f.MayRequireAny); err != nil {
					return err
				}
			}
		}
		var old []int64
		rows, err := tx.Query(`SELECT id FROM analysis WHERE environment_id=?`, envID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var x int64
			rows.Scan(&x)
			old = append(old, x)
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
		res, err := tx.Exec(`INSERT INTO analysis(environment_id, run_key, game_mm, version, started_at, finished_at, stats) VALUES(?,?,?,?,?,?,?)`,
			envID, runKey, mm, analyze.Version, started.Unix(), finished.Unix(), string(st))
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
		if err := saveHarmony(tx, out.Assemblies); err != nil {
			return err
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
		return nil
	})
	return id, err
}

// LatestAnalysis returns the environment's stored run (nil if none).
func (s *Store) LatestAnalysis(envID string) (*Analysis, error) {
	var a Analysis
	var st string
	var started, finished int64
	err := s.DB.QueryRow(`SELECT id, run_key, game_mm, version, started_at, finished_at, stats FROM analysis WHERE environment_id=? ORDER BY id DESC LIMIT 1`, envID).
		Scan(&a.ID, &a.RunKey, &a.MM, &a.Version, &started, &finished, &st)
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
	if f.XPathPart != "" {
		q += ` AND instr(xpath, ?) > 0`
		args = append(args, f.XPathPart)
	}
	if f.Target != "" {
		q += ` AND (' ' || targets || ' ' LIKE ? OR instr(leaf, ?) > 0)`
		name := f.Target
		if i := strings.IndexAny(name, "/@"); i >= 0 {
			name = name[i+1:]
		}
		args = append(args, "% "+f.Target+" %", `"`+name+`"`)
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

// Ignore is a problem the player chose to ignore (03 §4.2).
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
	s.DB.QueryRow(`SELECT count(*) FROM def_fact WHERE instance_id=?`, instanceID).Scan(&n)
	return n
}

// PatchCount returns an instance's operations and failures in a run.
func (s *Store) PatchCount(analysisID int64, instanceID string) (ops, failed int) {
	s.DB.QueryRow(`SELECT count(*), coalesce(sum(result='failed'), 0) FROM patch_fact WHERE analysis_id=? AND instance_id=?`, analysisID, instanceID).Scan(&ops, &failed)
	return
}

func saveHarmony(tx *sql.Tx, asms []*analyze.AssemblyFacts) error {
	seen := map[string]bool{}
	for _, a := range asms {
		if seen[a.Instance] {
			continue
		}
		seen[a.Instance] = true
		for _, t := range []string{"assembly_fact", "type_fact", "harmony_fact", "harmony_id"} {
			if _, err := tx.Exec(`DELETE FROM `+t+` WHERE instance_id=?`, a.Instance); err != nil {
				return err
			}
		}
	}
	insA, err := tx.Prepare(`INSERT INTO assembly_fact(instance_id, file, skipped, manual, uses_harmony, types, patches, error) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insA.Close()
	insT, err := tx.Prepare(`INSERT INTO type_fact(instance_id, file, type_name) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer insT.Close()
	insH, err := tx.Prepare(`INSERT INTO harmony_fact(instance_id, file, patch_type, patch_method, kind, target, target_type, target_method, args, skips, dynamic) VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insH.Close()
	insI, err := tx.Prepare(`INSERT INTO harmony_id(instance_id, file, id) VALUES(?,?,?)`)
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
	for _, a := range asms {
		if _, err := insA.Exec(a.Instance, a.File, b(a.Skipped), b(a.Manual), b(a.UsesHarm), len(a.Types), len(a.Patches), a.Err); err != nil {
			return err
		}
		for _, t := range a.Types {
			if _, err := insT.Exec(a.Instance, a.File, t); err != nil {
				return err
			}
		}
		for _, p := range a.Patches {
			if _, err := insH.Exec(a.Instance, a.File, p.PatchType, p.PatchMethod, p.Kind, p.Target(), p.TargetType, p.TargetMethod, p.Args, b(p.Skips), b(p.Dynamic)); err != nil {
				return err
			}
		}
		for _, id := range a.IDs {
			if _, err := insI.Exec(a.Instance, a.File, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// HarmonyRow is a stored Harmony patch.
type HarmonyRow struct {
	Instance, File, PatchType, PatchMethod, Kind, Target string
	Skips, Dynamic                                       bool
}

// HarmonyFilter selects Harmony patches.
type HarmonyFilter struct {
	Instance string
	Target   string // exact target (any overload), or with Like a case-insensitive substring
	Like     bool
}

// HarmonyPatches returns stored patches ordered by target.
func (s *Store) HarmonyPatches(f HarmonyFilter) ([]*HarmonyRow, error) {
	q := `SELECT instance_id, file, patch_type, patch_method, kind, target, skips, dynamic FROM harmony_fact WHERE 1=1`
	var args []any
	if f.Instance != "" {
		q += ` AND instance_id=?`
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
	rows, err := s.DB.Query(q+` ORDER BY target, rowid`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*HarmonyRow
	for rows.Next() {
		var h HarmonyRow
		var sk, dy int
		if err := rows.Scan(&h.Instance, &h.File, &h.PatchType, &h.PatchMethod, &h.Kind, &h.Target, &sk, &dy); err != nil {
			return nil, err
		}
		h.Skips, h.Dynamic = sk != 0, dy != 0
		out = append(out, &h)
	}
	return out, rows.Err()
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
	rows, err := s.DB.Query(`SELECT instance_id, file, skipped, manual, uses_harmony, types, patches, error FROM assembly_fact WHERE instance_id=? ORDER BY rowid`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AssemblyRow
	for rows.Next() {
		var a AssemblyRow
		var sk, ma, uh int
		if err := rows.Scan(&a.Instance, &a.File, &sk, &ma, &uh, &a.Types, &a.Patches, &a.Err); err != nil {
			return nil, err
		}
		a.Skipped, a.Manual, a.UsesHm = sk != 0, ma != 0, uh != 0
		out = append(out, &a)
	}
	return out, rows.Err()
}

// TypeOwners returns the (instance, file) pairs defining a type full name. Nested types are
// stored with '+'; stack frames may print either.
func (s *Store) TypeOwners(name string) ([][2]string, error) {
	alt := strings.ReplaceAll(name, ".", "+")
	rows, err := s.DB.Query(`SELECT DISTINCT instance_id, file FROM type_fact WHERE type_name IN (?, ?)`, name, alt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var a, b string
		rows.Scan(&a, &b)
		out = append(out, [2]string{a, b})
	}
	if len(out) == 0 && strings.Contains(name, ".") {
		// Nested types: try replacing the last dot(s) with '+' one at a time.
		parts := strings.Split(name, ".")
		for k := len(parts) - 1; k > 1 && len(out) == 0; k-- {
			cand := strings.Join(parts[:k], ".") + "+" + strings.Join(parts[k:], "+")
			r, err := s.TypeOwners0(cand)
			if err != nil {
				return nil, err
			}
			out = r
		}
	}
	return out, rows.Err()
}

// TypeOwners0 is TypeOwners for an exact stored name.
func (s *Store) TypeOwners0(name string) ([][2]string, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT instance_id, file FROM type_fact WHERE type_name=?`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var a, b string
		rows.Scan(&a, &b)
		out = append(out, [2]string{a, b})
	}
	return out, rows.Err()
}

// HarmonyIDOwners returns the instances whose assemblies contain the Harmony ID string.
func (s *Store) HarmonyIDOwners(id string) ([]string, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT instance_id FROM harmony_id WHERE id=?`, id)
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
