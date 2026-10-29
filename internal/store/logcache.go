package store

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"io"
)

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
	if isGzip(c.Report) {
		if c.Report, err = gunzipBytes(c.Report); err != nil {
			return nil, nil // unreadable: parse the log again
		}
	}
	return c, nil
}

// PutLogCache stores (replaces) the report of a log path. The JSON is stored gzipped: a
// report is often larger than its log (9 MB for a 4 MB Player.log) and compresses about 20×.
func (s *Store) PutLogCache(c *LogCache) error {
	z, err := gzipBytes(c.Report)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO log_cache(path, size, mtime, version, input_key, report) VALUES(?,?,?,?,?,?)
ON CONFLICT(path) DO UPDATE SET size=excluded.size, mtime=excluded.mtime, version=excluded.version, input_key=excluded.input_key, report=excluded.report`,
		c.Path, c.Size, c.MTime, c.Version, c.InputKey, z)
	return err
}

// isGzip tells a compressed report from the JSON stored before v25 (which starts with '{').
func isGzip(b []byte) bool { return len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b }

func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(b); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipBytes(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
