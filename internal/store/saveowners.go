package store

import "strings"

// Batch lookups for what a Mod owns in a save (10 §4.6): a save names thousands of Defs and
// hundreds of classes, so these take many names per query instead of one each (DefsNamed,
// TypeOwners). Like those, they read the version view of Store.MM: every instance with
// facts for that game version, in any environment.

// lookupChunk is how many names one query takes (well under SQLite's variable limit).
const lookupChunk = 500

// The batch queries for n names: the name index first, then the version view (CROSS
// JOIN keeps that order, as in DefsNamed).
func defDefinersQ(n int) string {
	return `SELECT DISTINCT f.def_name, c.instance_id, f.def_type FROM def_fact f CROSS JOIN content_state c ON c.def_set_id=f.set_id
 WHERE f.def_name IN (` + placeholders(n) + `) AND f.abstract=0 AND c.game_mm=? ORDER BY f.def_name, c.instance_id, f.def_type`
}

func typeDefinersQ(n int) string {
	return `SELECT DISTINCT f.type_name, c.instance_id FROM type_fact f CROSS JOIN content_state c ON c.dll_set_id=f.set_id
 WHERE f.type_name IN (` + placeholders(n) + `) AND c.game_mm=? ORDER BY f.type_name, c.instance_id`
}

// DefDefiner is one non-abstract Def fact defining a defName.
type DefDefiner struct {
	Instance string
	Type     string // the Def type (element name), e.g. ThingDef
}

// DefDefiners returns, for each of names that some instance defines, its non-abstract
// definitions: (instance, Def type), each pair once, by instance. Names nobody defines are
// absent from the map.
func (s *Store) DefDefiners(names []string) (map[string][]DefDefiner, error) {
	out := map[string][]DefDefiner{}
	err := chunked(names, func(part []string) error {
		rows, err := s.DB.Query(defDefinersQ(len(part)), append(anys(part), s.MM)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var d DefDefiner
			if err := rows.Scan(&name, &d.Instance, &d.Type); err != nil {
				return err
			}
			out[name] = append(out[name], d)
		}
		return rows.Err()
	})
	return out, err
}

// TypeDefiners returns, for each of the full type names (as stored: nested types with
// '+') that some instance's assemblies define, those instances. Case-sensitive, like
// TypeOwners.
func (s *Store) TypeDefiners(names []string) (map[string][]string, error) {
	out := map[string][]string{}
	err := chunked(names, func(part []string) error {
		rows, err := s.DB.Query(typeDefinersQ(len(part)), append(anys(part), s.MM)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name, inst string
			if err := rows.Scan(&name, &inst); err != nil {
				return err
			}
			out[name] = append(out[name], inst)
		}
		return rows.Err()
	})
	return out, err
}

// NestedTypeDefiners finds nested types by their own name: a save writes a type of one of
// SaveNamespaces by its Name, which for a nested type is the inner name only (07 §7 item
// 20), and type_fact has no simple-name column. It returns, for each of names, the
// instances defining a nested type "<ns>.Outer+…+<name>" whose outermost namespace is one
// of SaveNamespaces or the global one. One pass over Store.MM's nested type facts per call
// (compiler-generated types, whose names hold '<', are left out).
func (s *Store) NestedTypeDefiners(names []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(names) == 0 {
		return out, nil
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	ignored := map[string]bool{"": true}
	for _, ns := range SaveNamespaces {
		ignored[ns] = true
	}
	rows, err := s.DB.Query(`SELECT f.type_name, c.instance_id FROM content_state c JOIN type_fact f ON f.set_id=c.dll_set_id
 WHERE c.game_mm=? AND instr(f.type_name,'+')>0 AND instr(f.type_name,'<')=0`, s.MM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[[2]string]bool{}
	for rows.Next() {
		var full, inst string
		if err := rows.Scan(&full, &inst); err != nil {
			return nil, err
		}
		ns, name := typeNamespace(full)
		if !want[name] || !ignored[ns] || seen[[2]string{name, inst}] {
			continue
		}
		seen[[2]string{name, inst}] = true
		out[name] = append(out[name], inst)
	}
	return out, rows.Err()
}

// FactView says which facts of Store.MM an instance has.
type FactView struct {
	Defs bool // a Def view: analysed (enabled or not) for this game version
	DLLs bool // a DLL view: analysed while enabled, or unchanged since (03 §3)
}

// FactViews returns the views of Store.MM the instances have; an instance without any is
// absent.
func (s *Store) FactViews(instances []string) (map[string]FactView, error) {
	out := map[string]FactView{}
	err := chunked(instances, func(part []string) error {
		rows, err := s.DB.Query(`SELECT instance_id, dll_set_id IS NOT NULL FROM content_state WHERE game_mm=? AND instance_id IN (`+placeholders(len(part))+`)`,
			append([]any{s.MM}, anys(part)...)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var dll bool
			if err := rows.Scan(&id, &dll); err != nil {
				return err
			}
			out[id] = FactView{Defs: true, DLLs: dll}
		}
		return rows.Err()
	})
	return out, err
}

// PackageFactViews returns, by lowercase packageId, the views of Store.MM that any
// instance of that packageId has, in any environment and including tombstones (whose facts
// last until the version view is cleaned up, 03 §3): what is known of a Mod that is not
// installed here. Packages without any are absent.
func (s *Store) PackageFactViews(pkgs []string) (map[string]FactView, error) {
	out := map[string]FactView{}
	low := make([]string, len(pkgs))
	for i, p := range pkgs {
		low[i] = strings.ToLower(p)
	}
	err := chunked(low, func(part []string) error {
		rows, err := s.DB.Query(`SELECT lower(s.package_id), c.dll_set_id IS NOT NULL FROM content_state c JOIN instance i ON i.id=c.instance_id
 JOIN snapshot s ON s.id=i.current_snapshot_id WHERE c.game_mm=? AND lower(s.package_id) IN (`+placeholders(len(part))+`)`, append([]any{s.MM}, anys(part)...)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pkg string
			var dll bool
			if err := rows.Scan(&pkg, &dll); err != nil {
				return err
			}
			v := out[pkg]
			out[pkg] = FactView{Defs: true, DLLs: v.DLLs || dll}
		}
		return rows.Err()
	})
	return out, err
}

// InstancePackages returns the packageId (lowercase, as the snapshot declares it) and
// name of each instance's current snapshot, by instance; unknown instances are absent.
func (s *Store) InstancePackages(instances []string) (map[string][2]string, error) {
	out := map[string][2]string{}
	err := chunked(instances, func(part []string) error {
		rows, err := s.DB.Query(`SELECT i.id, lower(s.package_id), s.name FROM instance i JOIN snapshot s ON s.id=i.current_snapshot_id WHERE i.id IN (`+placeholders(len(part))+`)`, anys(part)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, pkg, name string
			if err := rows.Scan(&id, &pkg, &name); err != nil {
				return err
			}
			out[id] = [2]string{pkg, name}
		}
		return rows.Err()
	})
	return out, err
}

func chunked(names []string, fn func([]string) error) error {
	for len(names) > 0 {
		n := min(len(names), lookupChunk)
		if err := fn(names[:n]); err != nil {
			return err
		}
		names = names[n:]
	}
	return nil
}
