package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/content"
)

const schemaV26 = `
ALTER TABLE content_state RENAME TO content_state_v25;
ALTER TABLE def_fact RENAME TO def_fact_v25;
ALTER TABLE assembly_fact RENAME TO assembly_fact_v25;
ALTER TABLE type_fact RENAME TO type_fact_v25;
ALTER TABLE harmony_fact RENAME TO harmony_fact_v25;
ALTER TABLE harmony_id RENAME TO harmony_id_v25;
CREATE TABLE fact_set(
 id INTEGER PRIMARY KEY, instance_id TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('def','dll')),
 content_key TEXT NOT NULL, UNIQUE(instance_id,kind,content_key));
CREATE TABLE content_state(
 instance_id TEXT NOT NULL, game_mm TEXT NOT NULL, content_key TEXT NOT NULL, folders TEXT NOT NULL DEFAULT '',
 def_files INTEGER NOT NULL DEFAULT 0, patch_files INTEGER NOT NULL DEFAULT 0, parse_errors TEXT NOT NULL DEFAULT '',
 def_set_id INTEGER NOT NULL REFERENCES fact_set(id), dll_set_id INTEGER REFERENCES fact_set(id),
 PRIMARY KEY(instance_id,game_mm));
CREATE INDEX content_state_def ON content_state(def_set_id,game_mm);
CREATE INDEX content_state_dll ON content_state(dll_set_id,game_mm);
CREATE INDEX content_state_mm ON content_state(game_mm,instance_id);
CREATE TABLE def_fact(
 set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 def_type TEXT NOT NULL, def_name TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '',
 parent TEXT NOT NULL DEFAULT '', abstract INTEGER NOT NULL DEFAULT 0, file TEXT NOT NULL, line INTEGER NOT NULL,
 may_require TEXT NOT NULL DEFAULT '', may_require_any TEXT NOT NULL DEFAULT '');
CREATE INDEX def_fact_set ON def_fact(set_id);
CREATE INDEX def_fact_lookup ON def_fact(def_name,set_id);
CREATE INDEX def_fact_named ON def_fact(name,set_id) WHERE name<>'';
CREATE TABLE assembly_fact(
 set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 file TEXT NOT NULL, skipped INTEGER NOT NULL DEFAULT 0, manual INTEGER NOT NULL DEFAULT 0,
 uses_harmony INTEGER NOT NULL DEFAULT 0, types INTEGER NOT NULL DEFAULT 0, patches INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '');
CREATE INDEX assembly_fact_set ON assembly_fact(set_id);
CREATE TABLE type_fact(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE, file TEXT NOT NULL,type_name TEXT NOT NULL);
CREATE INDEX type_fact_set ON type_fact(set_id);
CREATE INDEX type_fact_lookup ON type_fact(type_name,set_id);
CREATE TABLE harmony_fact(
 set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 file TEXT NOT NULL,patch_type TEXT NOT NULL,patch_method TEXT NOT NULL,kind TEXT NOT NULL,
 target TEXT NOT NULL,target_type TEXT NOT NULL DEFAULT '',target_method TEXT NOT NULL DEFAULT '',args TEXT NOT NULL DEFAULT '',
 skips INTEGER NOT NULL DEFAULT 0,dynamic INTEGER NOT NULL DEFAULT 0);
CREATE INDEX harmony_fact_set ON harmony_fact(set_id);
CREATE TABLE harmony_id(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,file TEXT NOT NULL,id TEXT NOT NULL COLLATE NOCASE);
CREATE INDEX harmony_id_set ON harmony_id(set_id);
CREATE INDEX harmony_id_lookup ON harmony_id(id,set_id);
`

var migratedFactColumns = []struct{ table, columns string }{
	{"def_fact", "def_type,def_name,name,parent,abstract,file,line,may_require,may_require_any"},
	{"assembly_fact", "file,skipped,manual,uses_harmony,types,patches,error"},
	{"type_fact", "file,type_name"},
	{"harmony_fact", "file,patch_type,patch_method,kind,target,target_type,target_method,args,skips,dynamic"},
	{"harmony_id", "file,id"},
}

// migrateV26 converts cached inputs without reading mod files. Old DLL caches do not
// record their skip policy; a fingerprint preserves their exact view until reanalysis.
func migrateV26(tx *sql.Tx) error {
	type state struct {
		inst, mm, key, folders, errors string
		defs, patches                  int
	}
	var states []state
	rows, err := tx.Query(`SELECT instance_id,game_mm,content_key,folders,def_files,patch_files,parse_errors FROM content_state_v25 ORDER BY instance_id,game_mm`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c state
		if err := rows.Scan(&c.inst, &c.mm, &c.key, &c.folders, &c.defs, &c.patches, &c.errors); err != nil {
			rows.Close()
			return err
		}
		states = append(states, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range states {
		parts := strings.Split(c.key, "|")
		key := "legacy-def:" + c.mm + ":" + c.key
		if len(parts) >= 4 && strings.HasPrefix(parts[len(parts)-1], "a") {
			if snap, err := hex.DecodeString(parts[0]); err == nil {
				var folders []string
				if c.folders != "" {
					folders = strings.Split(c.folders, "\n")
				}
				key = content.FolderKey(snap, folders) + "|" + parts[len(parts)-1]
			}
		}
		defID, fresh, err := ensureFactSet(tx, c.inst, "def", key)
		if err != nil {
			return err
		}
		if fresh {
			if err := copyLegacyFacts(tx, migratedFactColumns[0].table, migratedFactColumns[0].columns, defID, c.inst, c.mm); err != nil {
				return err
			}
		}
		var dllID any
		hash := sha256.New()
		count := 0
		for _, f := range migratedFactColumns[1:] {
			rows, err := tx.Query(`SELECT json_array(`+f.columns+`) FROM `+f.table+`_v25 WHERE instance_id=? AND game_mm=? ORDER BY rowid`, c.inst, c.mm)
			if err != nil {
				return err
			}
			fmt.Fprintln(hash, f.table)
			for rows.Next() {
				var data string
				if err := rows.Scan(&data); err != nil {
					rows.Close()
					return err
				}
				fmt.Fprintln(hash, data)
				count++
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
		}
		if count > 0 {
			// Include snapshot and analyzer identity when available; identical DLL rows from
			// different snapshots still represent different cached inputs.
			identity := c.key
			if len(parts) >= 4 {
				identity = parts[0] + "|" + parts[len(parts)-1]
			}
			id, fresh, err := ensureFactSet(tx, c.inst, "dll", fmt.Sprintf("legacy-dll:%s|%x", identity, hash.Sum(nil)))
			if err != nil {
				return err
			}
			dllID = id
			if fresh {
				for _, f := range migratedFactColumns[1:] {
					if err := copyLegacyFacts(tx, f.table, f.columns, id, c.inst, c.mm); err != nil {
						return err
					}
				}
			}
		}
		if _, err := tx.Exec(`INSERT INTO content_state(instance_id,game_mm,content_key,folders,def_files,patch_files,parse_errors,def_set_id,dll_set_id) VALUES(?,?,?,?,?,?,?,?,?)`, c.inst, c.mm, key, c.folders, c.defs, c.patches, c.errors, defID, dllID); err != nil {
			return err
		}
	}
	for _, table := range []string{"content_state", "def_fact", "assembly_fact", "type_fact", "harmony_fact", "harmony_id"} {
		if _, err := tx.Exec(`DROP TABLE ` + table + `_v25`); err != nil {
			return err
		}
	}
	return nil
}

func copyLegacyFacts(tx *sql.Tx, table, columns string, id int64, inst, mm string) error {
	_, err := tx.Exec(`INSERT INTO `+table+`(set_id,`+columns+`) SELECT ?,`+columns+` FROM `+table+`_v25 WHERE instance_id=? AND game_mm=? ORDER BY rowid`, id, inst, mm)
	return err
}
