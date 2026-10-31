package store

import "sort"

// InstanceFiles returns the current files of the live instances an environment sees (the
// shared mod folders and its own Data/ and Mods/, 02 §0) whose relative path matches a LIKE
// pattern (ASCII case-insensitive), as instance ID → paths in path order. Log attribution
// uses it to find the installed mod that ships a missing texture or assembly (03 §6.2). A
// pattern for .dll files reads only manifest rows; any other also unpacks the current
// snapshots' blocks (02 §7).
func (s *Store) InstanceFiles(envID, like string) (map[string][]string, error) {
	rows, err := s.DB.Query(`SELECT i.id, f.rel_path FROM instance i
		JOIN snapshot_file f ON f.snapshot_id = i.current_snapshot_id
		WHERE i.environment_id IN ('', ?) AND i.tombstone_at IS NULL AND f.rel_path LIKE ?`, envID, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var id, rel string
		if err := rows.Scan(&id, &rel); err != nil {
			return nil, err
		}
		out[id] = append(out[id], rel)
	}
	if err := rows.Err(); err != nil || onlyDLLs(like) {
		return out, err
	}
	rows.Close()
	lists, err := s.DB.Query(`SELECT i.id, l.data FROM instance i
		JOIN snapshot_list l ON l.snapshot_id = i.current_snapshot_id
		WHERE i.environment_id IN ('', ?) AND i.tombstone_at IS NULL`, envID)
	if err != nil {
		return nil, err
	}
	defer lists.Close()
	for lists.Next() {
		var id string
		var data []byte
		if err := lists.Scan(&id, &data); err != nil {
			return nil, err
		}
		n := len(out[id])
		if err := decodeList(data, func(rel string, _ int64) {
			if likeMatch(like, rel) {
				out[id] = append(out[id], rel)
			}
		}); err != nil {
			return nil, err
		}
		if n > 0 && len(out[id]) > n {
			sort.Strings(out[id])
		}
	}
	return out, lists.Err()
}
