package store

// InstanceFiles returns the current files of an environment's live instances whose relative
// path matches a LIKE pattern (ASCII case-insensitive), as instance ID → paths. Log attribution
// uses it to find the installed mod that ships a missing texture or assembly.
func (s *Store) InstanceFiles(envID, like string) (map[string][]string, error) {
	rows, err := s.DB.Query(`SELECT i.id, f.rel_path FROM instance i
		JOIN snapshot_file f ON f.snapshot_id = i.current_snapshot_id
		WHERE i.environment_id=? AND i.tombstone_at IS NULL AND f.rel_path LIKE ?`, envID, like)
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
	return out, rows.Err()
}
