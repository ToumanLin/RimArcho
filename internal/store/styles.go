package store

import (
	"database/sql"
	"errors"
	"time"
)

// Styles (21 §5): the entries of the generated Mod RimArcho Styles Preference and the
// files RimArcho wrote into it. An entry applies to all plans.
const stylesSchema = `CREATE TABLE IF NOT EXISTS look_style (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	kind TEXT NOT NULL,
	look TEXT NOT NULL,
	lineage TEXT NOT NULL,
	target TEXT NOT NULL,
	random INTEGER NOT NULL,
	at INTEGER NOT NULL,
	UNIQUE(kind, look, lineage, target)
);
CREATE TABLE IF NOT EXISTS style_file (
	rel TEXT PRIMARY KEY,
	source TEXT NOT NULL,
	size INTEGER NOT NULL,
	mtime INTEGER NOT NULL,
	linked INTEGER NOT NULL
);
`

func upgradeStyles(tx *sql.Tx, _ func(int64, int64)) error {
	_, err := tx.Exec(stylesSchema)
	return err
}

// Kinds of a style entry (21 §5).
const (
	StyleKindStyle   = "style"   // a new ThingStyleDef in the style category Target
	StyleKindVariant = "variant" // a random variant of Target: "default" or a ThingStyleDef
)

// ErrStyleExists: the same entry (kind, look, Mod and target) is already stored.
var ErrStyleExists = errors.New("store: this style entry already exists")

// LookStyle is one entry: the Mod of Lineage's version of Look is kept as a style in the
// category Target, or as a random variant of Target.
type LookStyle struct {
	ID      int64 // never reused: the generated Def names carry it
	Kind    string
	Look    string
	Lineage string
	Target  string
	Random  bool // style: new items may receive it at random (the thing Def's randomStyle)
	At      time.Time
}

// LookStyles lists the entries in the order they were added.
func (s *Store) LookStyles() ([]LookStyle, error) {
	rows, err := s.DB.Query(`SELECT id, kind, look, lineage, target, random, at FROM look_style ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LookStyle
	for rows.Next() {
		var e LookStyle
		var at int64
		if err := rows.Scan(&e.ID, &e.Kind, &e.Look, &e.Lineage, &e.Target, &e.Random, &at); err != nil {
			return nil, err
		}
		e.At = time.Unix(at, 0).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddLookStyle stores a new entry and returns it with its ID; ErrStyleExists when the same
// kind, look, Mod and target is already there.
func (s *Store) AddLookStyle(e LookStyle) (LookStyle, error) {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM look_style WHERE kind=? AND look=? AND lineage=? AND target=?`,
		e.Kind, e.Look, e.Lineage, e.Target).Scan(&n); err != nil {
		return e, err
	}
	if n > 0 {
		return e, ErrStyleExists
	}
	r, err := s.DB.Exec(`INSERT INTO look_style(kind, look, lineage, target, random, at) VALUES(?,?,?,?,?,?)`,
		e.Kind, e.Look, e.Lineage, e.Target, e.Random, e.At.Unix())
	if err != nil {
		return e, err
	}
	e.ID, err = r.LastInsertId()
	e.At = time.Unix(e.At.Unix(), 0).UTC()
	return e, err
}

// DeleteLookStyle removes one entry; ok is false when there is none with that ID.
func (s *Store) DeleteLookStyle(id int64) (ok bool, err error) {
	r, err := s.DB.Exec(`DELETE FROM look_style WHERE id=?`, id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n > 0, err
}

// StyleFiles lists the files RimArcho wrote into RimArcho Styles Preference, ordered by Rel.
func (s *Store) StyleFiles() ([]LookFile, error) { return s.genFiles("style_file") }

// ReplaceStyleFiles makes fs the whole recorded set, in one transaction.
func (s *Store) ReplaceStyleFiles(fs []LookFile) error { return s.replaceGenFiles("style_file", fs) }
