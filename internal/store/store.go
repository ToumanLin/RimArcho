// Package store is RimArcho's SQLite fact store (02 §7), using modernc.org/sqlite in WAL mode
// with synchronous=NORMAL and a WAL size limit.
package store

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"
)

// SchemaVersion is the version schema.sql creates. A schema change bumps it, edits
// schema.sql and appends the step that upgrades the previous version to upgrades.
const SchemaVersion = 30

// FirstReleaseSchema is the schema of the first public release (1.0.0). Databases from
// development builds before it are not upgraded (02 §7).
const FirstReleaseSchema = 29

//go:embed schema.sql
var schema string

// upgrades[i] upgrades schema FirstReleaseSchema+i to the next version, inside one
// transaction; report gives progress for slow steps (Done of Total rows).
var upgrades = []func(tx *sql.Tx, report func(done, total int64)) error{
	upgradeGameTypes, // 29 → 30
}

// ErrPreRelease: the database was created by a development build before 1.0.0.
var ErrPreRelease = errors.New("store: database from a pre-1.0 development build")

// Shared is the environment ID of what every environment shares (02 §0): the mod folders
// and their instances, the lineages, the player's data on lineages and the user rules.
const Shared = ""

// Store wraps the database handle.
type Store struct {
	DB   *sql.DB
	Path string
	// MM is the game major.minor whose analysis facts the fact queries read (the app sets
	// it from the Loadout it works on).
	MM string

	setFiles *setFileCache // fact sets' file lists (setfiles.go); nil: read every time
}

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*Store, error) {
	return open(path, true, nil)
}

// UpgradeProgress is what OpenUpgrading reports while it upgrades an existing database
// from schema From to To (02 §7): Done of Total rows of the slow steps (v28), both 0 while
// the count is not known.
type UpgradeProgress struct {
	From, To    int
	Done, Total int64
}

// OpenUpgrading is Open that calls progress, on the opening goroutine, while it upgrades an
// existing database. A database that is new or already current makes no call.
func OpenUpgrading(path string, progress func(UpgradeProgress)) (*Store, error) {
	return open(path, true, progress)
}

// OpenSession connects to a database already initialised by Open. Page/worker
// sessions do not run migrations or negotiate the journal mode while a writer runs.
func OpenSession(path string) (*Store, error) {
	return open(path, false, nil)
}

// PeekMeta reads one meta value without upgrading or writing anything ("" when there is no
// database or no value). An upgrade on another connection does not block it (WAL).
func PeekMeta(path, key string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=busy_timeout(1000)")
	if err != nil {
		return ""
	}
	defer db.Close()
	var v string
	db.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	return v
}

func open(path string, initialise bool, progress func(UpgradeProgress)) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// Per-connection settings go in the DSN so every connection the pool opens gets them.
	// journal_size_limit: after a large write the WAL file is cut back to 16 MB when it is
	// next reset, instead of keeping its largest size while any connection is open (02 §7).
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_pragma=journal_size_limit(16777216)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if initialise {
		for _, p := range []string{"PRAGMA journal_mode=WAL"} {
			if _, err := db.Exec(p); err != nil {
				db.Close()
				return nil, fmt.Errorf("store: %s: %w", p, err)
			}
		}
	}
	s := &Store{DB: db, Path: path, setFiles: &setFileCache{}}
	if initialise {
		if err := s.migrate(progress); err != nil {
			db.Close()
			return nil, err
		}
	}
	if initialise {
		if err := s.installIndexTriggers(); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: index generation: %w", err)
		}
	}
	return s, nil
}

func (s *Store) Close() error {
	if s.setFiles != nil {
		s.setFiles.close()
	}
	return s.DB.Close()
}

func (s *Store) migrate(progress func(UpgradeProgress)) error {
	if len(upgrades) != SchemaVersion-FirstReleaseSchema {
		return fmt.Errorf("store: %d upgrade steps for schema %d..%d", len(upgrades), FirstReleaseSchema, SchemaVersion)
	}
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(schema); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: create schema: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', ?)`, fmt.Sprint(SchemaVersion)); err != nil {
			tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	v, _ := strconv.Atoi(s.Meta("schema_version"))
	if v >= 1 && v < FirstReleaseSchema {
		return fmt.Errorf("%w (schema %d); move %s away to start with a new database", ErrPreRelease, v, s.Path)
	}
	if v < 1 || v > SchemaVersion {
		return fmt.Errorf("store: unsupported schema version %d (want %d)", v, SchemaVersion)
	}
	from := v
	report := func(done, total int64) {
		if progress != nil {
			progress(UpgradeProgress{From: from, To: SchemaVersion, Done: done, Total: total})
		}
	}
	if v < SchemaVersion {
		report(0, 0)
	}
	for ; v < SchemaVersion; v++ {
		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		if err := upgrades[v-FirstReleaseSchema](tx, report); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migrate to v%d: %w", v+1, err)
		}
		if _, err := tx.Exec(`UPDATE meta SET value=? WHERE key='schema_version'`, fmt.Sprint(v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Meta reads a meta value ("" if absent).
func (s *Store) Meta(key string) string {
	var v string
	s.DB.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	return v
}

// SetMeta writes a meta value.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// ChangeKey changes whenever the database content may have changed: this connection's
// writes (total_changes) or a commit by another connection or process (data_version).
func (s *Store) ChangeKey() string {
	var dv, tc int64
	s.DB.QueryRow(`PRAGMA data_version`).Scan(&dv)
	s.DB.QueryRow(`SELECT total_changes()`).Scan(&tc)
	return fmt.Sprintf("%d.%d", dv, tc)
}
