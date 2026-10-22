package store

import "strings"

// What the changes page (06 §2.9) reads of stored snapshots: the declaration and Workshop
// time of the two ends of a mod's changes, and their file lists.

// SnapInfo is a stored snapshot's declaration and size.
type SnapInfo struct {
	ID             int64
	DeclBlob       string
	ACFTimeUpdated int64
	TotalSize      int64
}

// SnapInfos loads snapshots by ID (missing ones are left out).
func (s *Store) SnapInfos(ids []int64) (map[int64]*SnapInfo, error) {
	out := map[int64]*SnapInfo{}
	var args []any
	seen := map[int64]bool{}
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			args = append(args, id)
		}
	}
	for len(args) > 0 {
		n := min(len(args), 400)
		rows, err := s.DB.Query(`SELECT id, decl_blob, acf_time_updated, total_size FROM snapshot WHERE id IN (`+placeholders(n)+`)`, args[:n]...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var x SnapInfo
			if err := rows.Scan(&x.ID, &x.DeclBlob, &x.ACFTimeUpdated, &x.TotalSize); err != nil {
				rows.Close()
				return nil, err
			}
			out[x.ID] = &x
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		args = args[n:]
	}
	return out, nil
}

// SnapFile is one file of a stored snapshot: its path as found, size, and hash (semantic
// files only).
type SnapFile struct {
	Rel  string
	Size int64
	Hash []byte
}

// SnapshotFiles lists a snapshot's files keyed by lower-cased path (nil, nil when the
// snapshot has none stored).
func (s *Store) SnapshotFiles(id int64) (map[string]SnapFile, error) {
	rows, err := s.DB.Query(`SELECT rel_path, size, hash FROM snapshot_file WHERE snapshot_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out map[string]SnapFile
	for rows.Next() {
		var f SnapFile
		if err := rows.Scan(&f.Rel, &f.Size, &f.Hash); err != nil {
			return nil, err
		}
		if out == nil {
			out = map[string]SnapFile{}
		}
		out[strings.ToLower(f.Rel)] = f
	}
	return out, rows.Err()
}
