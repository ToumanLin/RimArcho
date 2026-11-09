package store

import "database/sql"

// The full read of a save (12 §1.7 L1) depends only on the file, so it is kept by path
// and valid while the size, modification time and parser version are unchanged. Lists
// read only the summary columns; the facts (gzip JSON) are read for one save at a time.
// Path is the caller's key for the file (app uses env.PathKey).

// SaveCache is one save's stored full read.
type SaveCache struct {
	Path   string
	Size   int64
	MTime  int64 // UnixNano
	Parser int   // savefile.ParserVersion
	SaveSummaryRow
	Facts []byte // JSON; nil when only the summary was read
}

// SaveSummaryRow is the part of the full read that lists and grouping need.
type SaveSummaryRow struct {
	Ticks     int64
	Identity  string // world identity, "" when the save holds none (12 §4.1)
	World     string // world name
	Faction   string // the player faction's name
	Malformed bool
}

// Valid reports whether the row still describes a file of this size and mtime read by a
// parser from minParser to parser.
func (c *SaveCache) Valid(size, mtime int64, minParser, parser int) bool {
	return c != nil && c.Size == size && c.MTime == mtime && c.Parser >= minParser && c.Parser <= parser
}

func upgradeSaveCache(tx *sql.Tx, _ func(done, total int64)) error {
	_, err := tx.Exec(`CREATE TABLE save_cache(
 path TEXT PRIMARY KEY, size INTEGER NOT NULL, mtime INTEGER NOT NULL, parser INTEGER NOT NULL,
 ticks INTEGER NOT NULL DEFAULT 0, identity TEXT NOT NULL DEFAULT '', world TEXT NOT NULL DEFAULT '',
 faction TEXT NOT NULL DEFAULT '', malformed INTEGER NOT NULL DEFAULT 0, facts BLOB NOT NULL);`)
	return err
}

// SaveSummaries returns every stored save's row without its facts, by path.
func (s *Store) SaveSummaries() (map[string]*SaveCache, error) {
	rows, err := s.DB.Query(`SELECT path, size, mtime, parser, ticks, identity, world, faction, malformed FROM save_cache`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*SaveCache{}
	for rows.Next() {
		c := &SaveCache{}
		if err := rows.Scan(&c.Path, &c.Size, &c.MTime, &c.Parser, &c.Ticks, &c.Identity, &c.World, &c.Faction, &c.Malformed); err != nil {
			return nil, err
		}
		out[c.Path] = c
	}
	return out, rows.Err()
}

// SaveFacts returns a save's stored row with its facts (nil if none or unreadable).
func (s *Store) SaveFacts(path string) (*SaveCache, error) {
	c := &SaveCache{Path: path}
	var z []byte
	err := s.DB.QueryRow(`SELECT size, mtime, parser, ticks, identity, world, faction, malformed, facts FROM save_cache WHERE path=?`, path).
		Scan(&c.Size, &c.MTime, &c.Parser, &c.Ticks, &c.Identity, &c.World, &c.Faction, &c.Malformed, &z)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if c.Facts, err = gunzipBytes(z); err != nil {
		return nil, nil // unreadable: read the save again
	}
	return c, nil
}

// PutSaveCache stores (replaces) a save's full read.
func (s *Store) PutSaveCache(c *SaveCache) error {
	z, err := gzipBytes(c.Facts)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO save_cache(path, size, mtime, parser, ticks, identity, world, faction, malformed, facts) VALUES(?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(path) DO UPDATE SET size=excluded.size, mtime=excluded.mtime, parser=excluded.parser, ticks=excluded.ticks,
 identity=excluded.identity, world=excluded.world, faction=excluded.faction, malformed=excluded.malformed, facts=excluded.facts`,
		c.Path, c.Size, c.MTime, c.Parser, c.Ticks, c.Identity, c.World, c.Faction, c.Malformed, z)
	return err
}

// DeleteSaveCache removes the rows of saves that no longer exist.
func (s *Store) DeleteSaveCache(paths []string) error {
	return s.inTx(func(tx *sql.Tx) error {
		for _, p := range paths {
			if _, err := tx.Exec(`DELETE FROM save_cache WHERE path=?`, p); err != nil {
				return err
			}
		}
		return nil
	})
}
