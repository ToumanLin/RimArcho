package store

import "database/sql"

// LogCache is a stored log report: valid while the file has the same size and mtime and
// the attribution inputs (InputKey) are unchanged.
type LogCache struct {
	Path     string
	Size     int64
	MTime    int64 // UnixNano
	Version  string
	InputKey string
	Report   []byte // JSON
}

// LogCache returns the stored report of a log path (nil if none).
func (s *Store) LogCache(path string) (*LogCache, error) {
	c := &LogCache{Path: path}
	err := s.DB.QueryRow(`SELECT size, mtime, version, input_key, report FROM log_cache WHERE path=?`, path).
		Scan(&c.Size, &c.MTime, &c.Version, &c.InputKey, &c.Report)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// PutLogCache stores (replaces) the report of a log path.
func (s *Store) PutLogCache(c *LogCache) error {
	_, err := s.DB.Exec(`INSERT INTO log_cache(path, size, mtime, version, input_key, report) VALUES(?,?,?,?,?,?)
ON CONFLICT(path) DO UPDATE SET size=excluded.size, mtime=excluded.mtime, version=excluded.version, input_key=excluded.input_key, report=excluded.report`,
		c.Path, c.Size, c.MTime, c.Version, c.InputKey, c.Report)
	return err
}
