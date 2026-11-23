package store

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
)

// Kept in Go so the fresh schema and the upgrade execute the same statement.
const saveReviewSchema = `
CREATE TABLE IF NOT EXISTS save_review_cache(
 path TEXT PRIMARY KEY REFERENCES save_cache(path) ON DELETE CASCADE,
 size INTEGER NOT NULL, mtime INTEGER NOT NULL, parser INTEGER NOT NULL,
 input_key TEXT NOT NULL, result BLOB NOT NULL);
`

func upgradeSaveReview(tx *sql.Tx, _ func(int64, int64)) error {
	_, err := tx.Exec(saveReviewSchema)
	return err
}

const saveReviewCatalogSchema = `CREATE TABLE IF NOT EXISTS save_review_catalog(key TEXT PRIMARY KEY, catalog BLOB NOT NULL);`
const saveReviewCatalogReference = `ALTER TABLE save_review_cache ADD COLUMN catalog_key TEXT REFERENCES save_review_catalog(key);
CREATE INDEX IF NOT EXISTS save_review_catalog_ref ON save_review_cache(catalog_key);`

func upgradeSaveReviewCatalog(tx *sql.Tx, _ func(int64, int64)) error {
	if _, err := tx.Exec(saveReviewCatalogSchema); err != nil {
		return err
	}
	// Some migration tests roll a current schema back to an earlier version. As
	// with the cache table itself, tolerate the current column already existing.
	rows, err := tx.Query(`SELECT name FROM pragma_table_info('save_review_cache') WHERE name='catalog_key'`)
	if err != nil {
		return err
	}
	have := rows.Next()
	err = rows.Err()
	rows.Close()
	if err != nil || have {
		return err
	}
	_, err = tx.Exec(saveReviewCatalogReference)
	return err
}

// SaveReviewCache is one save's compact attribution, not its full parser facts.
// Result is JSON on the caller side and gzip JSON on disk, as in SaveCache.
type SaveReviewCache struct {
	Path        string
	Size, MTime int64
	Parser      int
	InputKey    string
	Result      []byte
	CatalogKey  string
	Catalog     []byte // supplied only when publishing a newly built catalog
}

// SaveReviewCache reads only a current entry; corrupt compressed data is a miss.
func (s *Store) SaveReviewCache(path string, size, mtime int64, parser int, key string) ([]byte, error) {
	var z []byte
	err := s.DB.QueryRow(`SELECT result FROM save_review_cache WHERE path=? AND size=? AND mtime=? AND parser=? AND input_key=?`, path, size, mtime, parser, key).Scan(&z)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b, err := gunzipBytes(z)
	if err != nil {
		return nil, nil
	}
	return b, nil
}

func (s *Store) PutSaveReviewCache(c *SaveReviewCache) error {
	z, err := gzipBytes(c.Result)
	if err != nil {
		return err
	}
	var catalog []byte
	if c.Catalog != nil {
		if fmt.Sprintf("%x", sha256.Sum256(c.Catalog)) != c.CatalogKey {
			return fmt.Errorf("review catalog hash mismatch")
		}
		catalog, err = gzipBytes(c.Catalog)
		if err != nil {
			return err
		}
	}
	return s.inTx(func(tx *sql.Tx) error {
		var old sql.NullString
		err := tx.QueryRow(`SELECT catalog_key FROM save_review_cache WHERE path=?`, c.Path).Scan(&old)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if catalog != nil {
			if _, err := tx.Exec(`INSERT INTO save_review_catalog(key,catalog) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET catalog=excluded.catalog`, c.CatalogKey, catalog); err != nil {
				return err
			}
		}
		var key any
		if c.CatalogKey != "" {
			key = c.CatalogKey
		}
		_, err = tx.Exec(`INSERT INTO save_review_cache(path,size,mtime,parser,input_key,result,catalog_key) VALUES(?,?,?,?,?,?,?)
ON CONFLICT(path) DO UPDATE SET size=excluded.size,mtime=excluded.mtime,parser=excluded.parser,input_key=excluded.input_key,result=excluded.result,catalog_key=excluded.catalog_key`, c.Path, c.Size, c.MTime, c.Parser, c.InputKey, z, key)
		if err != nil {
			return err
		}
		if old.Valid && old.String != c.CatalogKey {
			_, err = tx.Exec(`DELETE FROM save_review_catalog WHERE key=? AND NOT EXISTS(SELECT 1 FROM save_review_cache WHERE catalog_key=?)`, old.String, old.String)
		}
		return err
	})
}

// SaveReviewCatalog verifies its content address. Missing or corrupt data is a
// cache miss, repaired together with the referencing save on its next review.
func (s *Store) SaveReviewCatalog(key string) ([]byte, error) {
	var z []byte
	err := s.DB.QueryRow(`SELECT catalog FROM save_review_catalog WHERE key=?`, key).Scan(&z)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b, err := gunzipBytes(z)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(b)) != key {
		return nil, nil
	}
	return b, nil
}

func pruneReviewCatalogs(tx *sql.Tx) error {
	_, err := tx.Exec(`DELETE FROM save_review_catalog WHERE NOT EXISTS(SELECT 1 FROM save_review_cache WHERE catalog_key=save_review_catalog.key)`)
	return err
}

// PruneSaveReviewFormat removes obsolete representations, including their large
// embedded catalogs, without touching the saves' full-read facts.
func (s *Store) PruneSaveReviewFormat(prefix string) error {
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM save_review_cache WHERE substr(input_key,1,?)<>?`, len(prefix), prefix); err != nil {
			return err
		}
		return pruneReviewCatalogs(tx)
	})
}

// ReviewContentKey covers immutable Def sets, their readiness and version views.
// IndexKey also covers installed-copy resolution and displayed metadata. A mere
// analysis run ID would invalidate unchanged facts; DefContentVersion alone would
// miss changed Mod content. Cache writes themselves affect neither input.
func (s *Store) ReviewContentKey() (string, error) {
	h := sha256.New()
	fmt.Fprintln(h, s.IndexKey())
	rows, err := s.DB.Query(`SELECT c.instance_id,c.game_mm,c.content_key,c.def_set_id,f.def_content_version FROM content_state c JOIN fact_set f ON f.id=c.def_set_id ORDER BY c.instance_id,c.game_mm`)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var inst, mm, key string
		var set int64
		var version int
		if err := rows.Scan(&inst, &mm, &key, &set, &version); err != nil {
			rows.Close()
			return "", err
		}
		fmt.Fprintf(h, "%q %q %q %d %d\n", inst, mm, key, set, version)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	// Aliases change which copy a packageId resolves to. The lists' workspaces do too,
	// but only for the saves attributed under that list: the app keys each save on its
	// own list's copies (app.reviewContext), so editing one list keeps every other save.
	for _, query := range []string{
		`SELECT instance_id,source_id,rel_path,real_path FROM instance_alias ORDER BY instance_id,source_id,rel_path`,
	} {
		rows, err := s.DB.Query(query)
		if err != nil {
			return "", err
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(cols))
			for i := range values {
				values[i] = new(string)
			}
			if err := rows.Scan(values...); err != nil {
				rows.Close()
				return "", err
			}
			for _, value := range values {
				fmt.Fprintf(h, "%q ", *value.(*string))
			}
			fmt.Fprintln(h)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
