package store

import (
	"database/sql"

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
