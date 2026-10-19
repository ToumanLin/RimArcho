package store

import (
	"database/sql"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
)

const metaCurrentEnv = "current_env"

// InsertEnv stores a new environment.
func (s *Store) InsertEnv(e *env.Environment) error {
	_, err := s.DB.Exec(`INSERT INTO environment(id,name,game_root,user_data_root,game_version,launch,created_at) VALUES(?,?,?,?,?,?,?)`,
		e.ID, e.Name, e.GameRoot, e.UserDataRoot, e.GameVersion, e.Launch, e.CreatedAt.Unix())
	return err
}

// UpdateEnv rewrites an environment's mutable fields.
func (s *Store) UpdateEnv(e *env.Environment) error {
	_, err := s.DB.Exec(`UPDATE environment SET name=?, game_root=?, user_data_root=?, game_version=?, launch=? WHERE id=?`,
		e.Name, e.GameRoot, e.UserDataRoot, e.GameVersion, e.Launch, e.ID)
	return err
}

func scanEnv(row interface{ Scan(...any) error }) (*env.Environment, error) {
	var e env.Environment
	var created int64
	if err := row.Scan(&e.ID, &e.Name, &e.GameRoot, &e.UserDataRoot, &e.GameVersion, &e.Launch, &created); err != nil {
		return nil, err
	}
	e.CreatedAt = time.Unix(created, 0).UTC()
	return &e, nil
}

const envCols = `id,name,game_root,user_data_root,game_version,launch,created_at`

// Envs lists environments by name.
func (s *Store) Envs() ([]*env.Environment, error) {
	rows, err := s.DB.Query(`SELECT ` + envCols + ` FROM environment ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*env.Environment
	for rows.Next() {
		e, err := scanEnv(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EnvByName returns the named environment, or nil.
func (s *Store) EnvByName(name string) (*env.Environment, error) {
	e, err := scanEnv(s.DB.QueryRow(`SELECT `+envCols+` FROM environment WHERE name=?`, name))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// EnvByID returns the environment, or nil.
func (s *Store) EnvByID(id string) (*env.Environment, error) {
	e, err := scanEnv(s.DB.QueryRow(`SELECT `+envCols+` FROM environment WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// CurrentEnv returns the environment selected with `env use`, or nil.
func (s *Store) CurrentEnv() (*env.Environment, error) {
	id := s.Meta(metaCurrentEnv)
	if id == "" {
		return nil, nil
	}
	return s.EnvByID(id)
}

// SetCurrentEnv selects the current environment.
func (s *Store) SetCurrentEnv(id string) error { return s.SetMeta(metaCurrentEnv, id) }

// Sources lists an environment's sources in insertion order of kinds.
func (s *Store) Sources(envID string) ([]*source.Source, error) {
	rows, err := s.DB.Query(`SELECT id,environment_id,kind,label,root,status,pending_root FROM source WHERE environment_id=?
		ORDER BY CASE kind WHEN 'official' THEN 0 WHEN 'local' THEN 1 WHEN 'workshop' THEN 2 WHEN 'steamcmd' THEN 3 ELSE 4 END, label`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*source.Source
	for rows.Next() {
		var src source.Source
		var kind string
		if err := rows.Scan(&src.ID, &src.EnvID, &kind, &src.Label, &src.Root, &src.Status, &src.PendingRoot); err != nil {
			return nil, err
		}
		src.Kind = source.Kind(kind)
		out = append(out, &src)
	}
	return out, rows.Err()
}

// InsertSource stores a new source.
func (s *Store) InsertSource(src *source.Source) error {
	_, err := s.DB.Exec(`INSERT INTO source(id,environment_id,kind,label,root,status,pending_root) VALUES(?,?,?,?,?,?,?)`,
		src.ID, src.EnvID, string(src.Kind), src.Label, src.Root, src.Status, src.PendingRoot)
	return err
}

// UpdateSource rewrites a source's root, status and pending root.
func (s *Store) UpdateSource(src *source.Source) error {
	_, err := s.DB.Exec(`UPDATE source SET root=?, status=?, pending_root=?, label=? WHERE id=?`,
		src.Root, src.Status, src.PendingRoot, src.Label, src.ID)
	return err
}

// SourceSemantics returns, for every instance of a source with a snapshot, its relative path
// (lower-cased) and semantic set — the "old" side of source re-binding (02 §3.2).
func (s *Store) SourceSemantics(sourceID string) (map[string]map[string][16]byte, error) {
	rows, err := s.DB.Query(`SELECT lower(i.rel_path), lower(f.rel_path), f.hash FROM instance i
		JOIN snapshot_file f ON f.snapshot_id = i.current_snapshot_id
		WHERE i.source_id=? AND f.hash IS NOT NULL`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string][16]byte{}
	for rows.Next() {
		var rel, file string
		var h []byte
		if err := rows.Scan(&rel, &file, &h); err != nil {
			return nil, err
		}
		m := out[rel]
		if m == nil {
			m = map[string][16]byte{}
			out[rel] = m
		}
		var hh [16]byte
		copy(hh[:], h)
		m[file] = hh
	}
	// Instances without semantic files still count as "old" entries.
	rows2, err := s.DB.Query(`SELECT lower(rel_path) FROM instance WHERE source_id=?`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var rel string
		rows2.Scan(&rel)
		if out[rel] == nil {
			out[rel] = map[string][16]byte{}
		}
	}
	return out, rows2.Err()
}

// DeleteEnv removes an environment and everything RimArcho stored for it. Only RimArcho's
// own database is touched; no game or mod file is.
func (s *Store) DeleteEnv(envID string) error {
	return s.inTx(func(tx *sql.Tx) error {
		inst := `(SELECT id FROM instance WHERE environment_id=?)`
		for _, q := range []string{
			`DELETE FROM snapshot_file WHERE snapshot_id IN (SELECT id FROM snapshot WHERE instance_id IN ` + inst + `)`,
			`DELETE FROM def_fact WHERE instance_id IN ` + inst,
			`DELETE FROM assembly_fact WHERE instance_id IN ` + inst,
			`DELETE FROM type_fact WHERE instance_id IN ` + inst,
			`DELETE FROM harmony_fact WHERE instance_id IN ` + inst,
			`DELETE FROM harmony_id WHERE instance_id IN ` + inst,
			`DELETE FROM content_state WHERE instance_id IN ` + inst,
			`DELETE FROM patch_fact WHERE analysis_id IN (SELECT id FROM analysis WHERE environment_id=?)`,
			`DELETE FROM finding WHERE analysis_id IN (SELECT id FROM analysis WHERE environment_id=?)`,
			`DELETE FROM analysis WHERE environment_id=?`,
			`DELETE FROM finding_ignore WHERE environment_id=?`,
			`DELETE FROM snapshot WHERE instance_id IN ` + inst,
			`DELETE FROM instance_alias WHERE instance_id IN ` + inst,
			`DELETE FROM change_event WHERE scan_id IN (SELECT id FROM scan WHERE environment_id=?)`,
			`DELETE FROM instance WHERE environment_id=?`,
			`DELETE FROM lineage WHERE environment_id=?`,
			`DELETE FROM identity_question WHERE environment_id=?`,
			`DELETE FROM scan WHERE environment_id=?`,
			`DELETE FROM source WHERE environment_id=?`,
			`DELETE FROM workspace WHERE environment_id=?`,
			`DELETE FROM workspace_undo WHERE environment_id=?`,
			`DELETE FROM loadout_version WHERE environment_id=?`,
			`DELETE FROM loadout_tag WHERE environment_id=?`,
			`DELETE FROM modsconfig_state WHERE environment_id=?`,
			`DELETE FROM user_rule WHERE environment_id=?`,
			`DELETE FROM lineage_data WHERE environment_id=?`,
			`DELETE FROM lineage_tag WHERE environment_id=?`,
			`DELETE FROM launch WHERE environment_id=?`,
			`DELETE FROM environment WHERE id=?`,
		} {
			if _, err := tx.Exec(q, envID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`DELETE FROM meta WHERE key=? AND value=?`, metaCurrentEnv, envID)
		return err
	})
}
