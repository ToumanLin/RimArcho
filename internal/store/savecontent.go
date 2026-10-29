package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/analyze"
)

func saveContent(tx *sql.Tx, out *analyze.Output, mm string) error {
	ins, err := tx.Prepare(`INSERT INTO def_fact(set_id,def_type,def_name,name,parent,abstract,file,line,may_require,may_require_any) VALUES(?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	grouped := map[string][]*analyze.AssemblyFacts{}
	for _, a := range out.Assemblies {
		grouped[a.Instance] = append(grouped[a.Instance], a)
	}
	dllSets := map[string]int64{}
	for inst, cs := range out.Content {
		defID, fresh, err := ensureFactSet(tx, inst, "def", cs.Key)
		if err != nil {
			return err
		}
		if fresh {
			fs, ok := out.NewDefs[inst]
			if !ok {
				return fmt.Errorf("missing rebuilt Def facts for %s", inst)
			}
			for _, f := range fs {
				if _, err := ins.Exec(defID, f.Type, f.DefName, f.Name, f.Parent, f.Abstract, f.File, f.Line, f.MayRequire, f.MayRequireAny); err != nil {
					return err
				}
			}
		}
		var dllID sql.NullInt64
		key := cs.AssemblyKey
		if key == "" && len(grouped[inst]) > 0 {
			// Supports producers of Output that supply assembly facts directly.
			data, err := json.Marshal(grouped[inst])
			if err != nil {
				return err
			}
			key = fmt.Sprintf("%s|output-dll%x", cs.Key, sha256.Sum256(data))
		}
		if key != "" {
			id, fresh, err := ensureFactSet(tx, inst, "dll", key)
			if err != nil {
				return err
			}
			dllID = sql.NullInt64{Int64: id, Valid: true}
			if fresh {
				dllSets[inst] = id
			}
		} else {
			err := tx.QueryRow(`SELECT dll_set_id FROM content_state WHERE instance_id=? AND game_mm=? AND content_key=?`, inst, mm, cs.Key).Scan(&dllID)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
		}
		if _, err := tx.Exec(`INSERT INTO content_state(instance_id,game_mm,content_key,folders,def_files,patch_files,parse_errors,def_set_id,dll_set_id) VALUES(?,?,?,?,?,?,?,?,?)
 ON CONFLICT(instance_id,game_mm) DO UPDATE SET content_key=excluded.content_key,folders=excluded.folders,def_files=excluded.def_files,patch_files=excluded.patch_files,parse_errors=excluded.parse_errors,def_set_id=excluded.def_set_id,dll_set_id=excluded.dll_set_id`, inst, mm, cs.Key, strings.Join(cs.Folders, "\n"), cs.DefFiles, cs.PatchFiles, strings.Join(cs.ParseErrors, "\n"), defID, dllID); err != nil {
			return err
		}
	}
	return saveHarmony(tx, out.Assemblies, dllSets)
}
