package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/analyze"
)

type analysisChunk func(func(*sql.Tx) error) error

func stageContent(chunk analysisChunk, run int64, out *analyze.Output) error {
	grouped := map[string][]*analyze.AssemblyFacts{}
	instances := make([]string, 0, len(out.Content))
	for _, a := range out.Assemblies {
		if out.Content[a.Instance] == nil {
			continue
		}
		if len(grouped[a.Instance]) == 0 {
			instances = append(instances, a.Instance)
		}
		grouped[a.Instance] = append(grouped[a.Instance], a)
	}
	for inst := range out.Content {
		if len(grouped[inst]) == 0 {
			instances = append(instances, inst)
		}
	}
	for _, inst := range instances {
		cs := out.Content[inst]
		fs, rebuilt := out.NewDefs[inst]
		linkDef := func(tx *sql.Tx, id int64) error {
			_, err := tx.Exec(`INSERT INTO analysis_stage_content(analysis_id,instance_id,content_key,folders,def_files,patch_files,parse_errors,def_set_id)
				VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(analysis_id,instance_id) DO UPDATE SET def_set_id=excluded.def_set_id`, run, inst,
				cs.Key, strings.Join(cs.Folders, "\n"), cs.DefFiles, cs.PatchFiles, strings.Join(cs.ParseErrors, "\n"), id)
			return err
		}
		if err := stageFactSet(chunk, inst, "def", cs.Key, len(fs), linkDef, func(exec factExec, id int64) error {
			if !rebuilt {
				return fmt.Errorf("missing rebuilt Def facts for %s", inst)
			}
			return saveDefs(exec, id, fs)
		}); err != nil {
			return err
		}
		asms := grouped[inst]
		key := cs.AssemblyKey
		if key == "" && len(asms) > 0 {
			// Supports producers of Output that supply assembly facts directly.
			data, err := json.Marshal(asms)
			if err != nil {
				return err
			}
			key = fmt.Sprintf("%s|output-dll%x", cs.Key, sha256.Sum256(data))
		}
		if key == "" {
			continue
		}
		rows := 0
		for _, a := range asms {
			rows += 1 + len(a.Types) + len(a.Patches) + len(a.IDs)
		}
		linkDLL := func(tx *sql.Tx, id int64) error {
			_, err := tx.Exec(`UPDATE analysis_stage_content SET dll_set_id=? WHERE analysis_id=? AND instance_id=?`, id, run, inst)
			return err
		}
		if err := stageFactSet(chunk, inst, "dll", key, rows, linkDLL, func(exec factExec, id int64) error {
			return saveHarmony(exec, asms, id)
		}); err != nil {
			return err
		}
	}
	return nil
}

// A large set owns a private key until every chunk has committed. Its staged
// reference keeps pruning away, and another completed set with the same key wins.
func stageFactSet(chunk analysisChunk, inst, kind, key string, rows int, link func(*sql.Tx, int64) error, save func(factExec, int64) error) error {
	if rows <= analysisChunkRows {
		return chunk(func(tx *sql.Tx) error {
			id, fresh, err := ensureFactSet(tx, inst, kind, key)
			if err != nil {
				return err
			}
			if fresh {
				if err := execFacts(tx, func(exec factExec) error { return save(exec, id) }); err != nil {
					return err
				}
			}
			return link(tx, id)
		})
	}
	privateKey := "\x00stage:" + rand.Text()
	var privateID int64
	if err := chunk(func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRow(`SELECT id FROM fact_set WHERE instance_id=? AND kind=? AND content_key=?`, inst, kind, key).Scan(&id)
		if err == sql.ErrNoRows {
			id, _, err = ensureFactSet(tx, inst, kind, privateKey)
			privateID = id
		}
		if err != nil {
			return err
		}
		return link(tx, id)
	}); err != nil || privateID == 0 {
		return err
	}
	var batch []factStatement
	flush := func() error {
		err := chunk(func(tx *sql.Tx) error {
			return execFacts(tx, func(exec factExec) error {
				for _, row := range batch {
					if err := exec(row.query, row.args...); err != nil {
						return err
					}
				}
				return nil
			})
		})
		clear(batch)
		batch = batch[:0]
		return err
	}
	if err := save(func(query string, args ...any) error {
		batch = append(batch, factStatement{query, args})
		if len(batch) == analysisChunkRows {
			return flush()
		}
		return nil
	}, privateID); err != nil {
		return err
	}
	if len(batch) > 0 {
		if err := flush(); err != nil {
			return err
		}
	}
	return chunk(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE OR IGNORE fact_set SET content_key=? WHERE id=?`, key, privateID); err != nil {
			return err
		}
		var id int64
		if err := tx.QueryRow(`SELECT id FROM fact_set WHERE instance_id=? AND kind=? AND content_key=?`, inst, kind, key).Scan(&id); err != nil {
			return err
		}
		return link(tx, id)
	})
}

type factExec func(query string, args ...any) error

type factStatement struct {
	query string
	args  []any
}

func execFacts(tx *sql.Tx, write func(factExec) error) error {
	stmts := map[string]*sql.Stmt{}
	defer func() {
		for _, stmt := range stmts {
			stmt.Close()
		}
	}()
	return write(func(query string, args ...any) error {
		stmt := stmts[query]
		if stmt == nil {
			var err error
			stmt, err = tx.Prepare(query)
			if err != nil {
				return err
			}
			stmts[query] = stmt
		}
		_, err := stmt.Exec(args...)
		return err
	})
}

func publishContent(tx *sql.Tx, run int64, mm string) error {
	_, err := tx.Exec(`INSERT INTO content_state(instance_id,game_mm,content_key,folders,def_files,patch_files,parse_errors,def_set_id,dll_set_id)
	SELECT stage.instance_id,?,stage.content_key,stage.folders,stage.def_files,stage.patch_files,stage.parse_errors,stage.def_set_id,
		coalesce(stage.dll_set_id,current.dll_set_id)
	FROM analysis_stage_content stage LEFT JOIN content_state current ON current.instance_id=stage.instance_id
		AND current.game_mm=? AND current.content_key=stage.content_key
	WHERE stage.analysis_id=?
	ON CONFLICT(instance_id,game_mm) DO UPDATE SET content_key=excluded.content_key,folders=excluded.folders,def_files=excluded.def_files,
		patch_files=excluded.patch_files,parse_errors=excluded.parse_errors,def_set_id=excluded.def_set_id,dll_set_id=excluded.dll_set_id`, mm, mm, run)
	return err
}

func saveDefs(exec factExec, defID int64, fs []analyze.DefFact) error {
	const insertDef = `INSERT INTO def_fact(set_id,def_type,def_name,name,parent,abstract,file_ix,line,may_require,may_require_any,content_fields) VALUES(?,?,?,?,?,?,?,?,?,?,?)`
	var files fileIndex
	ready := true
	for _, f := range fs {
		fields := ""
		if f.Content != nil {
			b, err := json.Marshal(f.Content)
			if err != nil {
				return err
			}
			fields = string(b)
		}
		ready = ready && f.Content != nil && f.Content.Version == analyze.DefContentVersion
		if err := exec(insertDef, defID, f.Type, f.DefName, f.Name, f.Parent, f.Abstract, files.of(f.File), f.Line, f.MayRequire, f.MayRequireAny, fields); err != nil {
			return err
		}
	}
	version := 0
	if ready {
		version = analyze.DefContentVersion
	}
	return exec(`UPDATE fact_set SET files=?,def_content_version=? WHERE id=?`, files.json(), version, defID)
}
