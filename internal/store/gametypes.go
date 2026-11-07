package store

import (
	"database/sql"
	"strings"
)

// The game's own managed assemblies (Assembly-CSharp.dll …) are indexed per environment at
// each scan, keyed by the environment's game major.minor, so lookups for Store.MM never mix
// 1.6 and 1.7 types (05 §5.3). Unlike Mod facts they have no fact set: an environment has
// one game folder, and a row is replaced when its file changes.

// GameMainAssembly is the game's own code; it defines every Verse and RimWorld type.
const GameMainAssembly = "Assembly-CSharp.dll"

// GameAssembly is one indexed managed assembly of an environment's game folder.
type GameAssembly struct {
	EnvID, File string
	MM          string // game major.minor when it was read
	Path        string
	Size        int64
	MTime       int64 // modification time, Unix nanoseconds
	Reader      int   // analyze.Version that read it
	Types       int   // type names stored
	Err         string
}

// Unchanged reports whether a stored assembly still describes g: same file, game version
// and reader, so its type names need not be read again.
func (a *GameAssembly) Unchanged(g *GameAssembly) bool {
	return a.Path == g.Path && a.Size == g.Size && a.MTime == g.MTime && a.Reader == g.Reader && a.MM == g.MM
}

func upgradeGameTypes(tx *sql.Tx, _ func(done, total int64)) error {
	_, err := tx.Exec(`CREATE TABLE game_assembly(
 id INTEGER PRIMARY KEY, environment_id TEXT NOT NULL, file TEXT NOT NULL, game_mm TEXT NOT NULL,
 path TEXT NOT NULL, size INTEGER NOT NULL, mtime INTEGER NOT NULL, reader INTEGER NOT NULL,
 types INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '', UNIQUE(environment_id,file));
CREATE TABLE game_type(
 assembly_id INTEGER NOT NULL REFERENCES game_assembly(id) ON DELETE CASCADE,
 type_name TEXT NOT NULL COLLATE NOCASE, ns TEXT NOT NULL, name TEXT NOT NULL);
CREATE INDEX game_type_assembly ON game_type(assembly_id);
CREATE INDEX game_type_lookup ON game_type(type_name,assembly_id);
CREATE INDEX game_type_named ON game_type(name,assembly_id);`)
	return err
}

// GameAssemblies returns the environment's indexed game assemblies by file name.
func (s *Store) GameAssemblies(envID string) (map[string]*GameAssembly, error) {
	rows, err := s.DB.Query(`SELECT file, game_mm, path, size, mtime, reader, types, error FROM game_assembly WHERE environment_id=?`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*GameAssembly{}
	for rows.Next() {
		g := &GameAssembly{EnvID: envID}
		if err := rows.Scan(&g.File, &g.MM, &g.Path, &g.Size, &g.MTime, &g.Reader, &g.Types, &g.Err); err != nil {
			return nil, err
		}
		out[g.File] = g
	}
	return out, rows.Err()
}

// SaveGameAssembly replaces the environment's row for g.File and its type names (full
// names as dotnet.Assembly.TypeNames gives them, nested types with '+'). g.Types is set.
func (s *Store) SaveGameAssembly(g *GameAssembly, types []string) error {
	g.Types = len(types)
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM game_assembly WHERE environment_id=? AND file=?`, g.EnvID, g.File); err != nil {
			return err
		}
		r, err := tx.Exec(`INSERT INTO game_assembly(environment_id, file, game_mm, path, size, mtime, reader, types, error) VALUES(?,?,?,?,?,?,?,?,?)`,
			g.EnvID, g.File, g.MM, g.Path, g.Size, g.MTime, g.Reader, g.Types, g.Err)
		if err != nil {
			return err
		}
		id, err := r.LastInsertId()
		if err != nil {
			return err
		}
		ins, err := tx.Prepare(`INSERT INTO game_type(assembly_id, type_name, ns, name) VALUES(?,?,?,?)`)
		if err != nil {
			return err
		}
		defer ins.Close()
		for _, t := range types {
			ns, name := typeNamespace(t)
			if _, err := ins.Exec(id, t, ns, name); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteGameAssembly removes the environment's row for file and its type names.
func (s *Store) DeleteGameAssembly(envID, file string) error {
	_, err := s.DB.Exec(`DELETE FROM game_assembly WHERE environment_id=? AND file=?`, envID, file)
	return err
}

// typeNamespace splits a full type name the way System.Type does: the namespace of the
// outermost type, and the simple name of the type itself ("RimWorld.A+B" → "RimWorld", "B").
func typeNamespace(full string) (ns, name string) {
	outer, _, _ := strings.Cut(full, "+")
	if i := strings.LastIndexByte(outer, '.'); i >= 0 {
		ns = outer[:i]
	}
	name = full[strings.LastIndexAny(full, ".+")+1:]
	return ns, name
}

// GameTypeOwners returns the game assembly files (e.g. "Assembly-CSharp.dll") that define
// a type full name in a game of version Store.MM; empty when none does or no game of that
// version has been scanned (GameTypesIndexed). Nested types are found when printed with
// dots too, as in TypeOwners. Unlike TypeOwners the match ignores case, as the game's own
// type lookup does (17 §6).
func (s *Store) GameTypeOwners(name string) ([]string, error) {
	var out []string
	err := nestedForms(name, func(names ...string) (bool, error) {
		var err error
		out, err = s.gameTypeFiles(`t.type_name IN (`+placeholders(len(names))+`)`, anys(names)...)
		return len(out) > 0, err
	})
	return out, err
}

// SaveNamespaces are the namespaces whose types a save names without the namespace
// (Verse.GenTypes.IgnoredNamespaceNames, 1.6.4633; 17 §6). "" is the global namespace,
// which the game searches by simple name too.
var SaveNamespaces = []string{"RimWorld", "Verse", "LudeonTK", "Verse.AI", "Verse.AI.Group", "Verse.Sound",
	"Verse.Grammar", "RimWorld.Planet", "RimWorld.BaseGen", "RimWorld.QuestGen", "RimWorld.SketchGen", "System"}

// GameClassOwners resolves a save's Class="…" value against the game's own assemblies of
// version Store.MM in the game's order (GenTypes.GetTypeInAnyAssembly, 17 §6): the simple
// name of a type in one of SaveNamespaces or the global namespace (how the game writes those,
// nested types by their own name), else the full name (GameTypeOwners), else the name
// prefixed by each of SaveNamespaces. A generic value resolves by its definition
// ("Verse.Pair`2[…]" → "Verse.Pair`2"); its arguments are separate lookups. Types of Mod
// assemblies are not consulted: a caller asks TypeOwners for those.
func (s *Store) GameClassOwners(class string) ([]string, error) {
	if i := strings.IndexByte(class, '['); i > 0 {
		class = class[:i]
	}
	if !strings.ContainsAny(class, ".+") {
		args := append([]any{class, ""}, anys(SaveNamespaces)...)
		out, err := s.gameTypeFiles(`t.name=? AND t.ns IN (?,`+placeholders(len(SaveNamespaces))+`)`, args...)
		if len(out) > 0 || err != nil {
			return out, err
		}
	}
	out, err := s.GameTypeOwners(class)
	if len(out) > 0 || err != nil {
		return out, err
	}
	names := make([]string, len(SaveNamespaces))
	for i, ns := range SaveNamespaces {
		names[i] = ns + "." + class
	}
	return s.gameTypeFiles(`t.type_name IN (`+placeholders(len(names))+`)`, anys(names)...)
}

// GameTypesIndexed reports whether a game of version Store.MM has its main assembly
// indexed. Until then (no such game scanned since this version of RimArcho, or its
// Assembly-CSharp.dll unreadable) an empty GameTypeOwners says nothing about the type.
func (s *Store) GameTypesIndexed() (bool, error) {
	var ok bool
	err := s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM game_assembly WHERE game_mm=? AND file=? AND types>0)`, s.MM, GameMainAssembly).Scan(&ok)
	return ok, err
}

// gameTypeFiles lists the distinct files of the matching game types for Store.MM.
func (s *Store) gameTypeFiles(where string, args ...any) ([]string, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT a.file FROM game_type t CROSS JOIN game_assembly a ON a.id=t.assembly_id WHERE `+where+` AND a.game_mm=? ORDER BY a.file`, append(args, s.MM)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
