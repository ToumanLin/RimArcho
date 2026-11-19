package store

import (
	"database/sql"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
)

const metaCurrentEnv = "current_env"

// InsertEnv stores an environment. An omitted GameID creates its installation with
// the same ID, preserving the existing one-environment registration contract.
func (s *Store) InsertEnv(e *env.Environment) error {
	gameID := e.GameID
	if gameID == "" {
		gameID = e.ID
	}
	err := s.inTx(func(tx *sql.Tx) error {
		if e.GameID == "" {
			if err := insertGame(tx, &Game{ID: gameID, Name: e.Name, GameRoot: e.GameRoot, GameVersion: e.GameVersion, CreatedAt: e.CreatedAt}); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`INSERT INTO environment(id,name,game_id,user_data_root,launch,created_at) VALUES(?,?,?,?,?,?)`, e.ID, e.Name, gameID, e.UserDataRoot, e.Launch, e.CreatedAt.Unix())
		return err
	})
	if err == nil {
		e.GameID = gameID
	}
	return err
}

// UpdateEnv rewrites only environment settings. Installation fields use UpdateGame.
func (s *Store) UpdateEnv(e *env.Environment) error {
	_, err := s.DB.Exec(`UPDATE environment SET name=?, user_data_root=?, launch=? WHERE id=?`, e.Name, e.UserDataRoot, e.Launch, e.ID)
	return err
}

func scanEnv(row interface{ Scan(...any) error }) (*env.Environment, error) {
	var e env.Environment
	var created int64
	if err := row.Scan(&e.ID, &e.GameID, &e.Name, &e.GameRoot, &e.UserDataRoot, &e.GameVersion, &e.Launch, &created); err != nil {
		return nil, err
	}
	e.CreatedAt = time.Unix(created, 0).UTC()
	return &e, nil
}

const envCols = `e.id,e.game_id,e.name,g.game_root,e.user_data_root,g.game_version,e.launch,e.created_at`
const envFrom = ` FROM environment e JOIN game g ON g.id=e.game_id`

// Envs lists environments by name.
func (s *Store) Envs() ([]*env.Environment, error) {
	rows, err := s.DB.Query(`SELECT ` + envCols + envFrom + ` ORDER BY e.name`)
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
	e, err := scanEnv(s.DB.QueryRow(`SELECT `+envCols+envFrom+` WHERE e.name=?`, name))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// EnvByID returns the environment, or nil.
func (s *Store) EnvByID(id string) (*env.Environment, error) {
	e, err := scanEnv(s.DB.QueryRow(`SELECT `+envCols+envFrom+` WHERE e.id=?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// EnvForGame returns a representative environment for operations on an installation's
// physical inventory. It does not select launch settings or a current plan.
func (s *Store) EnvForGame(gameID string) (*env.Environment, error) {
	e, err := scanEnv(s.DB.QueryRow(`SELECT `+envCols+envFrom+` WHERE g.id=? ORDER BY e.name LIMIT 1`, gameID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// CurrentEnv returns the environment selected with `env use`, or nil.
func (s *Store) CurrentEnv() (*env.Environment, error) {
	var id string
	if err := s.DB.QueryRow(`SELECT value FROM meta WHERE key=?`, metaCurrentEnv).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	return s.EnvByID(id)
}

// SetCurrentEnv selects the current environment.
func (s *Store) SetCurrentEnv(id string) error { return s.SetMeta(metaCurrentEnv, id) }

// Sources lists the sources an environment sees: its own (official, local) and the shared
// mod folders (03 §2), by kind, then label.
func (s *Store) Sources(gameID string) ([]*source.Source, error) {
	return s.sources(`game_id IN ('', ?)`, gameID)
}

// SharedSources lists the shared mod folders (every environment sees them).
func (s *Store) SharedSources() ([]*source.Source, error) { return s.sources(`game_id=''`) }

// AllSources lists every source of every environment and the shared ones.
func (s *Store) AllSources() ([]*source.Source, error) { return s.sources(`1`) }

func (s *Store) sources(where string, args ...any) ([]*source.Source, error) {
	rows, err := s.DB.Query(`SELECT id,game_id,kind,label,root,status,pending_root FROM source WHERE `+where+`
		ORDER BY CASE kind WHEN 'official' THEN 0 WHEN 'local' THEN 1 WHEN 'workshop' THEN 2 WHEN 'steamcmd' THEN 3 ELSE 4 END, label`, args...)
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
	_, err := s.DB.Exec(`INSERT INTO source(id,game_id,kind,label,root,status,pending_root) VALUES(?,?,?,?,?,?,?)`,
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
// (lower-cased) and semantic set — the "old" side of source re-binding (03 §4.2).
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

// DeleteEnv forgets one environment and, only for its last environment, its
// installation and inventory. Global plans and shared folders stay; no files are touched.
func (s *Store) DeleteEnv(envID string) error {
	return s.inTx(func(tx *sql.Tx) error {
		var gameID, name string
		err := tx.QueryRow(`SELECT game_id,name FROM environment WHERE id=?`, envID).Scan(&gameID, &name)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		var wasCurrent bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM meta WHERE key=? AND value=?)`, metaCurrentEnv, envID).Scan(&wasCurrent); err != nil {
			return err
		}
		for _, q := range []string{`DELETE FROM launch WHERE environment_id=?`, `DELETE FROM finding_ignore WHERE environment_id=?`, `DELETE FROM environment WHERE id=?`} {
			if _, err := tx.Exec(q, envID); err != nil {
				return err
			}
		}
		if err := deleteEnvMeta(tx, envID, name); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM meta WHERE key=? AND value=?`, metaCurrentEnv, envID); err != nil {
			return err
		}
		if wasCurrent {
			// The next current environment is chosen in the same transaction, so an
			// interruption never leaves environments without a current one: a sibling of
			// the same installation first, else the first by name (app.EnvRemoval.Next).
			if _, err := tx.Exec(`INSERT OR REPLACE INTO meta(key,value) SELECT ?,id FROM environment ORDER BY game_id<>?, name LIMIT 1`, metaCurrentEnv, gameID); err != nil {
				return err
			}
		}
		var remaining int
		if err := tx.QueryRow(`SELECT count(*) FROM environment WHERE game_id=?`, gameID).Scan(&remaining); err != nil {
			return err
		}
		if remaining != 0 {
			return nil
		}
		if err := deleteInstallation(tx, gameID); err != nil {
			return err
		}
		return pruneFacts(tx, time.Now())
	})
}

// deleteEnvMeta forgets the meta values kept per environment (keyed by its ID, or by its
// name for the Workshop client's update check). Profile import mappings
// (import:rimcrow:profile:*) stay on purpose: they keep a removed import from returning.
func deleteEnvMeta(tx *sql.Tx, envID, name string) error {
	if _, err := tx.Exec(`DELETE FROM meta WHERE key IN (?,?,?,?)`, "env:"+envID+":launch_args", "log.selected:"+envID, "log.seen:"+envID, "ui.columns:"+envID); err != nil {
		return err
	}
	prefix := "memory-estimate:" + envID + ":"
	if _, err := tx.Exec(`DELETE FROM meta WHERE substr(key,1,length(?))=?`, prefix, prefix); err != nil {
		return err
	}
	// Shared folders' subscription checks use the "shared" scope, which an environment
	// named "shared" must not take with it.
	prefix = "subscriptions:check:" + name + ":"
	_, err := tx.Exec(`DELETE FROM meta WHERE substr(key,1,length(?))=? AND key NOT IN (?,?,?)`, prefix, prefix,
		"subscriptions:check:shared:steamcmd", "subscriptions:check:shared:git", "subscriptions:check:shared:page")
	return err
}

// LiveCounts counts the live instances of each source.
func (s *Store) LiveCounts() (map[string]int, error) {
	rows, err := s.DB.Query(`SELECT source_id, count(*) FROM instance WHERE status='live' GROUP BY source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

func deleteInstallation(tx *sql.Tx, gameID string) error {
	if err := keepLists(tx, gameID); err != nil {
		return err
	}
	inst := `(SELECT id FROM instance WHERE game_id=?)`
	for _, q := range []string{
		`DELETE FROM snapshot_file WHERE snapshot_id IN (SELECT id FROM snapshot WHERE instance_id IN ` + inst + `)`,
		`DELETE FROM snapshot_list WHERE snapshot_id IN (SELECT id FROM snapshot WHERE instance_id IN ` + inst + `)`,
		`DELETE FROM content_state WHERE instance_id IN ` + inst,
		`DELETE FROM instance_note WHERE instance_id IN ` + inst,
		`DELETE FROM patch_fact WHERE analysis_id IN (SELECT id FROM analysis WHERE game_id=?)`,
		`DELETE FROM finding WHERE analysis_id IN (SELECT id FROM analysis WHERE game_id=?)`,
		`DELETE FROM analysis WHERE game_id=?`,
		`DELETE FROM snapshot WHERE instance_id IN ` + inst,
		`DELETE FROM instance_alias WHERE instance_id IN ` + inst,
		`DELETE FROM instance_alias WHERE source_id IN (SELECT id FROM source WHERE game_id=?)`,
		`DELETE FROM change_event WHERE instance_id IN ` + inst,
		`DELETE FROM identity_question WHERE game_id=? OR instance_id IN ` + inst + ` OR candidate_instance_id IN ` + inst,
		`DELETE FROM instance WHERE game_id=?`,
		`DELETE FROM scan WHERE game_id=? AND id NOT IN (SELECT scan_id FROM change_event)`,
		`UPDATE scan SET game_id='' WHERE game_id=?`,
		`DELETE FROM source WHERE game_id=?`,
		// The ModsConfig.xml write record belongs to the user data folder, which other
		// environments may share; it stays.
		// Lists are shared by every game and stay (07 §1.1); keepLists already named
		// this game's own mods in them by packageId.
		`DELETE FROM game_assembly WHERE game_id=?`, // and its game_type rows
		`DELETE FROM game WHERE id=?`,
	} {
		args := make([]any, strings.Count(q, "?"))
		for i := range args {
			args[i] = gameID
		}
		if _, err := tx.Exec(q, args...); err != nil {
			return err
		}
	}
	for _, q := range []string{
		`DELETE FROM lineage WHERE id NOT IN (SELECT lineage_id FROM instance)`,
		`DELETE FROM lineage_data WHERE lineage_id NOT IN (SELECT id FROM lineage)`,
		`DELETE FROM lineage_tag WHERE lineage_id NOT IN (SELECT id FROM lineage)`,
		`DELETE FROM lineage_functional WHERE lineage_id NOT IN (SELECT id FROM lineage)`,
		`DELETE FROM user_rule WHERE (a_lineage<>'' AND a_lineage NOT IN (SELECT id FROM lineage)) OR (b_lineage<>'' AND b_lineage NOT IN (SELECT id FROM lineage))`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// SourceBytes is each source's recorded size: the sum of its live Mods' sizes from their last
// scan (16 §2.14 Folders headings). Nothing on the disk is read.
func (s *Store) SourceBytes() (map[string]int64, error) {
	rows, err := s.DB.Query(`SELECT i.source_id, coalesce(sum(n.total_size),0) FROM instance i JOIN snapshot n ON n.id=i.current_snapshot_id WHERE i.status='live' GROUP BY i.source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
