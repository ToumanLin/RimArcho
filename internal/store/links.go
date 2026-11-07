package store

type ManagedLink struct {
	ID         int64  `json:"id"`
	Operation  string `json:"operation"`
	Phase      string `json:"phase"`
	Mods       string `json:"mods"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Instance   string `json:"instance,omitempty"`
	Pack       string `json:"pack,omitempty"`
	Target     string `json:"target"`
	NextPath   string `json:"next_path"`
	NextTarget string `json:"next_target"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	Directory  string `json:"directory,omitempty"`
}

func (s *Store) RecordLinkIntent(links []ManagedLink) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, l := range links {
		if _, err := tx.Exec(`INSERT INTO managed_link(operation_id,phase,mods_path,name,kind,instance_id,pack_id,target,next_path,next_target,from_path,to_path,directory_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, l.Operation, "intent", l.Mods, l.Name, l.Kind, l.Instance, l.Pack, l.Target, l.NextPath, l.NextTarget, l.From, l.To, l.Directory); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LinkPhase(operation, phase string) error {
	_, err := s.DB.Exec(`UPDATE managed_link SET phase=? WHERE operation_id=?`, phase, operation)
	return err
}

func (s *Store) LinkEntryPhase(operation, mods, name, phase string) error {
	_, err := s.DB.Exec(`UPDATE managed_link SET phase=? WHERE operation_id=? AND mods_path=? AND name=?`, phase, operation, mods, name)
	return err
}

func (s *Store) ManagedLinks() ([]ManagedLink, error) {
	rows, err := s.DB.Query(`SELECT id,operation_id,phase,mods_path,name,kind,instance_id,pack_id,target,next_path,next_target,from_path,to_path,directory_id FROM managed_link ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedLink
	for rows.Next() {
		var l ManagedLink
		if err := rows.Scan(&l.ID, &l.Operation, &l.Phase, &l.Mods, &l.Name, &l.Kind, &l.Instance, &l.Pack, &l.Target, &l.NextPath, &l.NextTarget, &l.From, &l.To, &l.Directory); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func LinkFinished(phase string) bool { return phase == "complete" || phase == "rolled-back" }
