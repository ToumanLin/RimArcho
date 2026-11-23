package store

// OfficialLineages are the lineages of gameID's own live instances in the game's Data/
// folder: Core and the DLCs (16 §3.8, the RimWorld tag).
func (s *Store) OfficialLineages(gameID string) ([]string, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT i.lineage_id FROM instance i JOIN source s ON s.id = i.source_id
		WHERE i.game_id = ? AND s.kind = 'official' AND i.status = 'live' AND i.lineage_id != ''
		ORDER BY i.lineage_id`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
