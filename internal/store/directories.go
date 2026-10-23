package store

import (
	"database/sql"
	"path/filepath"

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
	old := directoryCanonical(src.Root)
	realRoot := directoryCanonical(root)
	err := s.inTx(func(tx *sql.Tx) error {
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
	})
	if err == nil {
		src.Root, src.Status, src.PendingRoot = root, source.StatusOK, ""
	}
	return err
}
