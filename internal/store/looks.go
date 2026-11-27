package store

import (
	"database/sql"
	"strconv"
	"strings"
	"time"
)

// Looks (21 §3.1, §4): the player's choices of which Mod a look comes from, and the files
// RimArcho wrote into the generated Mod _Archo_Looks.
const looksSchema = `CREATE TABLE IF NOT EXISTS look_choice (
	scope TEXT NOT NULL,
	look TEXT NOT NULL,
	slot TEXT NOT NULL,
	lineage TEXT NOT NULL,
	at INTEGER NOT NULL,
	PRIMARY KEY(scope, look, slot)
);
CREATE TABLE IF NOT EXISTS look_file (
	rel TEXT PRIMARY KEY,
	source TEXT NOT NULL,
	size INTEGER NOT NULL,
	mtime INTEGER NOT NULL,
	linked INTEGER NOT NULL
);
`

func upgradeLooks(tx *sql.Tx, _ func(int64, int64)) error {
	_, err := tx.Exec(looksSchema)
	return err
}

// LookScopeAll is the scope of a choice for all plans (21 §3.1).
const LookScopeAll = "all"

// LookScope is the scope of a choice for one plan.
func LookScope(planID int64) string { return strconv.FormatInt(planID, 10) }

// LookChoice is one choice: the look, or one slot of it (Slot "all" = the whole look),
// comes from the Mod of Lineage.
type LookChoice struct {
	Scope   string // LookScopeAll or LookScope(plan)
	Look    string
	Slot    string
	Lineage string
	At      time.Time
}

// LookChoices lists the choices of the given scopes (none given: every scope), ordered by
// scope, look and slot.
func (s *Store) LookChoices(scopes ...string) ([]LookChoice, error) {
	q := `SELECT scope, look, slot, lineage, at FROM look_choice`
	var args []any
	if len(scopes) > 0 {
		q += ` WHERE scope IN (?` + strings.Repeat(",?", len(scopes)-1) + `)`
		for _, sc := range scopes {
			args = append(args, sc)
		}
	}
	rows, err := s.DB.Query(q+` ORDER BY scope, look, slot`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LookChoice
	for rows.Next() {
		var c LookChoice
		var at int64
		if err := rows.Scan(&c.Scope, &c.Look, &c.Slot, &c.Lineage, &at); err != nil {
			return nil, err
		}
		c.At = time.Unix(at, 0).UTC()
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetLookChoices writes the choices, one row per scope, look and slot, in one transaction.
// With whole set, the look's other rows in that scope are removed first: choosing the
// entire look replaces its per-slot choices.
func (s *Store) SetLookChoices(cs []LookChoice, whole bool) error {
	return s.inTx(func(tx *sql.Tx) error {
		for _, c := range cs {
			if whole {
				if _, err := tx.Exec(`DELETE FROM look_choice WHERE scope=? AND look=?`, c.Scope, c.Look); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`INSERT INTO look_choice(scope, look, slot, lineage, at) VALUES(?,?,?,?,?)
				ON CONFLICT(scope, look, slot) DO UPDATE SET lineage=excluded.lineage, at=excluded.at`,
				c.Scope, c.Look, c.Slot, c.Lineage, c.At.Unix()); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteLookChoices removes one look's choices in one scope; n is how many rows went.
func (s *Store) DeleteLookChoices(scope, look string) (n int64, err error) {
	r, err := s.DB.Exec(`DELETE FROM look_choice WHERE scope=? AND look=?`, scope, look)
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

// LookFile is one file RimArcho wrote into _Archo_Looks (21 §4).
type LookFile struct {
	Rel    string // relative to the generated Mod's folder, forward slashes
	Source string // the Mod's file it links to or was copied from
	Size   int64
	MTime  int64 // the source's, unix nanoseconds
	Linked bool  // a hard link; false: a copy
}

// LookFiles lists the recorded files, ordered by Rel.
func (s *Store) LookFiles() ([]LookFile, error) {
	rows, err := s.DB.Query(`SELECT rel, source, size, mtime, linked FROM look_file ORDER BY rel`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LookFile
	for rows.Next() {
		var f LookFile
		if err := rows.Scan(&f.Rel, &f.Source, &f.Size, &f.MTime, &f.Linked); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ReplaceLookFiles makes fs the whole recorded set, in one transaction.
func (s *Store) ReplaceLookFiles(fs []LookFile) error {
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM look_file`); err != nil {
			return err
		}
		for _, f := range fs {
			if _, err := tx.Exec(`INSERT INTO look_file(rel, source, size, mtime, linked) VALUES(?,?,?,?,?)`,
				f.Rel, f.Source, f.Size, f.MTime, f.Linked); err != nil {
				return err
			}
		}
		return nil
	})
}
