package store

import (
	"database/sql"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// Game is one installation, independently of its environments' user data and launch settings.
type Game struct {
	ID, Name, GameRoot, GameVersion string
	CreatedAt                       time.Time
}

func gameRootKey(root string) string { return env.PathKey(directoryCanonical(root)) }

func insertGame(tx *sql.Tx, g *Game) error {
	_, err := tx.Exec(`INSERT INTO game(id,name,game_root,game_root_key,game_version,created_at) VALUES(?,?,?,?,?,?)`,
		g.ID, g.Name, g.GameRoot, gameRootKey(g.GameRoot), g.GameVersion, g.CreatedAt.Unix())
	return err
}

// InsertGame stores an installation before any environments reference it.
func (s *Store) InsertGame(g *Game) error {
	return s.inTx(func(tx *sql.Tx) error { return insertGame(tx, g) })
}

func updateGame(tx *sql.Tx, g *Game) error {
	_, err := tx.Exec(`UPDATE game SET name=?, game_root=?, game_root_key=?, game_version=? WHERE id=?`,
		g.Name, g.GameRoot, gameRootKey(g.GameRoot), g.GameVersion, g.ID)
	return err
}

// UpdateGame changes installation fields without changing environment settings.
func (s *Store) UpdateGame(g *Game) error {
	return s.inTx(func(tx *sql.Tx) error { return updateGame(tx, g) })
}

// GameByID returns an installation, or nil.
func (s *Store) GameByID(id string) (*Game, error) {
	var g Game
	var created int64
	err := s.DB.QueryRow(`SELECT id,name,game_root,game_version,created_at FROM game WHERE id=?`, id).
		Scan(&g.ID, &g.Name, &g.GameRoot, &g.GameVersion, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	g.CreatedAt = time.Unix(created, 0).UTC()
	return &g, err
}

// Games lists installation labels once for inventory projections across games.
func (s *Store) Games() ([]*Game, error) {
	rows, err := s.DB.Query(`SELECT id,name,game_root,game_version,created_at FROM game ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Game
	for rows.Next() {
		var g Game
		var created int64
		if err := rows.Scan(&g.ID, &g.Name, &g.GameRoot, &g.GameVersion, &created); err != nil {
			return nil, err
		}
		g.CreatedAt = time.Unix(created, 0).UTC()
		out = append(out, &g)
	}
	return out, rows.Err()
}

func upgradeGameEnvironment(tx *sql.Tx, report func(done, total int64)) error {
	if _, err := tx.Exec(`CREATE TABLE game(
  id TEXT PRIMARY KEY, name TEXT NOT NULL, game_root TEXT NOT NULL,
  game_root_key TEXT NOT NULL, game_version TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL);
CREATE INDEX game_root_key ON game(game_root_key);
INSERT INTO game SELECT id,name,game_root,'',game_version,created_at FROM environment;`); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT id,game_root FROM game`)
	if err != nil {
		return err
	}
	type root struct{ id, path string }
	var roots []root
	for rows.Next() {
		var r root
		if err := rows.Scan(&r.id, &r.path); err != nil {
			rows.Close()
			return err
		}
		roots = append(roots, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for i, r := range roots {
		if _, err := tx.Exec(`UPDATE game SET game_root_key=? WHERE id=?`, gameRootKey(r.path), r.id); err != nil {
			return err
		}
		report(int64(i+1), int64(len(roots)))
	}
	// Legacy aliases keep distinct identities even when their canonical keys coincide.
	if _, err := tx.Exec(`CREATE TABLE environment_v38(
  id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, game_id TEXT NOT NULL REFERENCES game(id),
  user_data_root TEXT NOT NULL DEFAULT '', launch TEXT NOT NULL DEFAULT 'direct',
  created_at INTEGER NOT NULL, current_plan INTEGER NOT NULL DEFAULT 0);
INSERT INTO environment_v38 SELECT id,name,id,user_data_root,launch,created_at,current_plan FROM environment;
DROP TABLE environment;
ALTER TABLE environment_v38 RENAME TO environment;
CREATE INDEX environment_game ON environment(game_id);`); err != nil {
		return err
	}
	for _, table := range []string{"source", "instance", "scan", "identity_question", "analysis", "game_assembly"} {
		if _, err := tx.Exec(`ALTER TABLE ` + table + ` RENAME COLUMN environment_id TO game_id`); err != nil {
			return err
		}
	}
	_, err = tx.Exec(indexTriggerSQL())
	if err != nil {
		return err
	}
	// Table replacement must also invalidate indexes built before the upgrade.
	_, err = tx.Exec(`UPDATE archo_index_gen SET n=n+1`)
	return err
}
