package store

import (
	"database/sql"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
)

func directoryCanonical(path string) string {
	if real, err := source.RealPath(path); err == nil {
		return real
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(directoryCanonical(parent), filepath.Base(path))
}

// RebindSource accepts the same source at a new root. Instance IDs, snapshots and
// plan entries stay intact; stored real paths (including aliases from other sources)
// follow the directory when they were inside the old root.
func (s *Store) RebindSource(src *source.Source, root string) error {
	err := s.inTx(func(tx *sql.Tx) error { return rebindSource(tx, src, root) })
	if err == nil {
		src.Root, src.Status, src.PendingRoot = root, source.StatusOK, ""
	}
	return err
}

func rebindSource(tx *sql.Tx, src *source.Source, root string) error {
	old := directoryCanonical(src.Root)
	realRoot := directoryCanonical(root)
	for _, table := range []string{"instance", "instance_alias"} {
		rows, err := tx.Query(`SELECT rowid, real_path FROM ` + table)
		if err != nil {
			return err
		}
		type change struct {
			id   int64
			path string
		}
		var changes []change
		for rows.Next() {
			var id int64
			var path string
			if err := rows.Scan(&id, &path); err != nil {
				rows.Close()
				return err
			}
			if path != "" && source.Within(path, old) {
				rel, err := filepath.Rel(old, path)
				if err != nil {
					rows.Close()
					return err
				}
				changes = append(changes, change{id, filepath.Join(realRoot, rel)})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, c := range changes {
			if _, err := tx.Exec(`UPDATE `+table+` SET real_path=? WHERE rowid=?`, c.path, c.id); err != nil {
				return err
			}
		}
	}
	_, err := tx.Exec(`UPDATE source SET root=?, status=?, pending_root='' WHERE id=?`, root, source.StatusOK, src.ID)
	return err
}

// SourceMove is one source change of a move that MoveRecords applies with the others.
type SourceMove struct {
	Src *source.Source
	// Rebind: the same source at this root (RebindSource). Empty: Src is written as given,
	// inserted when Insert is set (a new source, or a rebind left pending).
	Rebind string
	Insert bool
}

// MoveRecords applies a game or directory move's records in one transaction: the
// environment row (when e is set), its source changes and meta values, so an error leaves
// none of them written (03 §4.2).
func (s *Store) MoveRecords(e *env.Environment, moves []SourceMove, meta map[string]string) error {
	err := s.inTx(func(tx *sql.Tx) error {
		if e != nil {
			if _, err := tx.Exec(`UPDATE environment SET name=?, game_root=?, user_data_root=?, game_version=?, launch=? WHERE id=?`,
				e.Name, e.GameRoot, e.UserDataRoot, e.GameVersion, e.Launch, e.ID); err != nil {
				return err
			}
		}
		for _, m := range moves {
			var err error
			switch {
			case m.Rebind != "":
				err = rebindSource(tx, m.Src, m.Rebind)
			case m.Insert:
				_, err = tx.Exec(`INSERT INTO source(id,environment_id,kind,label,root,status,pending_root) VALUES(?,?,?,?,?,?,?)`,
					m.Src.ID, m.Src.EnvID, string(m.Src.Kind), m.Src.Label, m.Src.Root, m.Src.Status, m.Src.PendingRoot)
			default:
				_, err = tx.Exec(`UPDATE source SET root=?, status=?, pending_root=?, label=? WHERE id=?`,
					m.Src.Root, m.Src.Status, m.Src.PendingRoot, m.Src.Label, m.Src.ID)
			}
			if err != nil {
				return err
			}
		}
		for k, v := range meta {
			if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		for _, m := range moves {
			if m.Rebind != "" {
				m.Src.Root, m.Src.Status, m.Src.PendingRoot = m.Rebind, source.StatusOK, ""
			}
		}
	}
	return err
}
