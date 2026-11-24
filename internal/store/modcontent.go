package store

import (
	"database/sql"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// SnapContent is what a snapshot's stored file list and About.xml say about a Mod's content,
// the facts of its automatic category (app.CategoryOf, 16 §2.5). Snapshots never change
// (04 §4), so callers may keep it by snapshot ID.
type SnapContent struct {
	Kinds   int  // snapshot.kinds: HasDLL, HasDefsPatch, HasLanguages, HasPNG
	HasDefs bool // XML under a Defs/ folder, at any load-folder depth
	// Requires are the packageIds its About.xml requires, for any game version, lower case
	// and without _steam.
	Requires []string
	// Defs, Patches and DLLs count those files; only SnapshotCounts fills them (one Mod's
	// details), since counting every row of a large library is slow (18 §2).
	Defs, Patches, DLLs int
}

// defsRow is a hashed file row (snapshot_file keeps every Defs/Patches XML and DLL, 04 §4)
// holding Defs XML; SQLite's LIKE folds ASCII case as the scan's own kinds do.
const defsRow = `f.rel_path LIKE '%.xml' AND (f.rel_path LIKE 'defs/%' OR f.rel_path LIKE '%/defs/%')`

// SnapshotContent reads the content facts of these snapshots by primary key: the kinds and
// the declared requirements, and for those with Defs or Patches XML whether any of it is
// Defs (an EXISTS over the snapshot's file rows, which stops at the first). Nothing reads Mod
// folders.
func (s *Store) SnapshotContent(ids []int64) (map[int64]*SnapContent, error) {
	out := make(map[int64]*SnapContent, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		if id != 0 && out[id] == nil {
			out[id] = &SnapContent{}
			args = append(args, id)
		}
	}
	err := s.eachSnapshot(args, `kinds, decl_blob, CASE WHEN kinds & `+strconv.Itoa(HasDefsPatch)+` = 0 THEN 0
		ELSE EXISTS(SELECT 1 FROM snapshot_file f WHERE f.snapshot_id = snapshot.id AND `+defsRow+`) END`, func(r *sql.Rows) error {
		var id int64
		var k, defs int
		var blob string
		if err := r.Scan(&id, &k, &blob, &defs); err != nil {
			return err
		}
		c := out[id]
		c.Kinds, c.HasDefs, c.Requires = k, defs != 0, declRequires(blob)
		return nil
	})
	return out, err
}

// SnapshotCounts counts one snapshot's Defs XML, Patches XML and DLL files (a Mod's
// category reason in its details).
func (s *Store) SnapshotCounts(id int64) (defs, patches, dlls int, err error) {
	err = s.DB.QueryRow(`SELECT coalesce(sum(`+defsRow+`), 0),
		coalesce(sum(f.rel_path LIKE '%.xml' AND (f.rel_path LIKE 'patches/%' OR f.rel_path LIKE '%/patches/%')), 0),
		coalesce(sum(f.rel_path LIKE '%.dll'), 0)
		FROM snapshot_file f WHERE f.snapshot_id = ?`, id).Scan(&defs, &patches, &dlls)
	return
}

// declRequires is the packageIds a stored declaration requires, any game version, each once.
func declRequires(blob string) []string {
	if !strings.Contains(blob, `"dependencies`) {
		return nil
	}
	type dep struct {
		PackageID string `json:"package_id"`
	}
	var d struct {
		Deps      []dep            `json:"dependencies"`
		ByVersion map[string][]dep `json:"dependencies_by_version"`
	}
	if json.Unmarshal([]byte(blob), &d) != nil {
		return nil
	}
	var out []string
	add := func(ds []dep) {
		for _, x := range ds {
			p := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(x.PackageID)), "_steam")
			if p != "" && !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	add(d.Deps)
	for _, ds := range d.ByVersion {
		add(ds)
	}
	return out
}
