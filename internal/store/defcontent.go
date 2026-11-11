package store

import (
	"database/sql"
	"encoding/json"

	"github.com/ToumanLin/RimArcho/internal/analyze"
)

func upgradeDefContent(tx *sql.Tx, _ func(int64, int64)) error {
	_, err := tx.Exec(`ALTER TABLE def_fact ADD COLUMN content_fields TEXT NOT NULL DEFAULT '';
ALTER TABLE fact_set ADD COLUMN def_content_version INTEGER NOT NULL DEFAULT 0`)
	return err
}

// ContentReady checks extraction version per Mod, including Mods with no Defs.
// Existing views remain valid for all other fact readers during the background rebuild.
func (s *Store) ContentReady(instances []string) (map[string]bool, error) {
	out := map[string]bool{}
	err := chunked(instances, func(part []string) error {
		rows, err := s.DB.Query(`SELECT c.instance_id,f.def_content_version FROM content_state c JOIN fact_set f ON f.id=c.def_set_id WHERE c.game_mm=? AND c.instance_id IN (`+placeholders(len(part))+`)`, append([]any{s.MM}, anys(part)...)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var v int
			if err := rows.Scan(&id, &v); err != nil {
				return err
			}
			out[id] = v == analyze.DefContentVersion
		}
		return rows.Err()
	})
	return out, err
}

// ContentDefs reads selected names in batches. Parent lookup is Name-keyed; this is
// intentionally distinct from defName and includes abstract nodes (12 §2.2).
func (s *Store) ContentDefs(instances, names []string, parent bool) ([]analyze.DefFact, error) {
	var out []analyze.DefFact
	if len(instances) == 0 {
		return out, nil
	}
	col := "f.def_name"
	if parent {
		col = "f.name"
	}
	err := chunked(instances, func(insts []string) error {
		return chunked(names, func(part []string) error {
			args := append(anys(part), s.MM)
			args = append(args, anys(insts)...)
			rows, err := s.DB.Query(`SELECT `+defCols+defByName+`WHERE `+col+` IN (`+placeholders(len(part))+`) AND c.game_mm=? AND c.instance_id IN (`+placeholders(len(insts))+`) ORDER BY f.rowid`, args...)
			if err != nil {
				return err
			}
			fs, err := s.scanDefs(rows)
			out = append(out, fs...)
			return err
		})
	})
	return out, err
}

// DefsOfTypes reads the Defs of the given types that instances provide for the current
// game version.
func (s *Store) DefsOfTypes(instances, types []string) ([]analyze.DefFact, error) {
	var out []analyze.DefFact
	err := chunked(instances, func(part []string) error {
		args := append([]any{s.MM}, anys(part)...)
		rows, err := s.DB.Query(`SELECT `+defCols+defView+`WHERE c.game_mm=? AND c.instance_id IN (`+placeholders(len(part))+`) AND f.def_type IN (`+placeholders(len(types))+`) ORDER BY c.instance_id, f.rowid`, append(args, anys(types)...)...)
		if err != nil {
			return err
		}
		fs, err := s.scanDefs(rows)
		out = append(out, fs...)
		return err
	})
	return out, err
}

// DefsLite reads the Defs of one type that instances provide for the current game version,
// without their files, and with their content fields only where those mention field: enough
// to walk ParentName chains for one inherited field over tens of thousands of Defs.
func (s *Store) DefsLite(instances []string, typ, field string) ([]analyze.DefFact, error) {
	var out []analyze.DefFact
	err := chunked(instances, func(part []string) error {
		args := append([]any{"%" + field + "%", s.MM}, anys(part)...)
		rows, err := s.DB.Query(`SELECT c.instance_id,f.def_name,f.name,f.parent,f.abstract,f.may_require,f.may_require_any,CASE WHEN f.content_fields LIKE ? THEN f.content_fields ELSE '' END`+defView+`WHERE c.game_mm=? AND c.instance_id IN (`+placeholders(len(part))+`) AND f.def_type=? ORDER BY c.instance_id, f.rowid`, append(args, typ)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			f := analyze.DefFact{Type: typ}
			var abs int
			var fields string
			if err := rows.Scan(&f.Instance, &f.DefName, &f.Name, &f.Parent, &abs, &f.MayRequire, &f.MayRequireAny, &fields); err != nil {
				return err
			}
			if fields != "" && json.Unmarshal([]byte(fields), &f.Content) != nil {
				f.Content = nil
			}
			f.Abstract = abs != 0
			out = append(out, f)
		}
		return rows.Err()
	})
	return out, err
}
