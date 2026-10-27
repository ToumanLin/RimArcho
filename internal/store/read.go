package store

import "sync"

// ReadSnapshot pins one WAL view while an exclusively owned page session builds
// its result. DB has one connection; all its queries use that same transaction.
// This takes no write lock. The caller must finish initialisation/imports first,
// perform only reads until release, and release before publishing the result.
func (s *Store) ReadSnapshot() (release func(), err error) {
	if _, err := s.DB.Exec(`BEGIN`); err != nil {
		return nil, err
	}
	// Establish the snapshot before another connection can commit an editor change.
	if _, err := s.DB.Exec(`SELECT value FROM meta WHERE key='schema_version'`); err != nil {
		s.DB.Exec(`ROLLBACK`)
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { s.DB.Exec(`ROLLBACK`) }) }, nil
}
