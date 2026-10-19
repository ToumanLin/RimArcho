package store

// Content kinds of snapshots (06 §5.5 type:assembly|xml|language), from their file lists.
const (
	HasDLL       = 1 << iota // a .dll anywhere
	HasDefsPatch             // XML under Defs/ or Patches/ (at any load-folder depth)
	HasLanguages             // something under Languages/
)

// ContentKinds returns the content flags of the current snapshots of the given instances
// (instance ID → flags), in one grouped query over snapshot_file.
func (s *Store) ContentKinds(snapOf map[string]int64) (map[string]int, error) {
	rows, err := s.DB.Query(`SELECT snapshot_id,
	  MAX(rel_path LIKE '%.dll'),
	  MAX((rel_path LIKE 'defs/%' OR rel_path LIKE '%/defs/%' OR rel_path LIKE 'patches/%' OR rel_path LIKE '%/patches/%') AND rel_path LIKE '%.xml'),
	  MAX(rel_path LIKE 'languages/%' OR rel_path LIKE '%/languages/%')
	FROM snapshot_file GROUP BY snapshot_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bySnap := map[int64]int{}
	for rows.Next() {
		var id int64
		var dll, dp, lang int
		if err := rows.Scan(&id, &dll, &dp, &lang); err != nil {
			return nil, err
		}
		f := 0
		if dll == 1 {
			f |= HasDLL
		}
		if dp == 1 {
			f |= HasDefsPatch
		}
		if lang == 1 {
			f |= HasLanguages
		}
		bySnap[id] = f
	}
	out := make(map[string]int, len(snapOf))
	for inst, snap := range snapOf {
		out[inst] = bySnap[snap]
	}
	return out, rows.Err()
}
