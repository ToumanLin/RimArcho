package store

import (
	"strings"
	"sync"
)

// Content kinds of snapshots (06 §3.7 type:assembly|xml|language), from their file lists.
const (
	HasDLL       = 1 << iota // a .dll anywhere
	HasDefsPatch             // XML under Defs/ or Patches/ (at any load-folder depth)
	HasLanguages             // something under Languages/
)

// ContentKinds returns the content flags of the current snapshots of the given instances
// (instance ID → flags), in one grouped query over snapshot_file.
//
// A snapshot's files keep their paths for its whole life (a later scan with other files makes
// a new snapshot), so its flags are kept per database and only new snapshots are queried.
func (s *Store) ContentKinds(snapOf map[string]int64) (map[string]int, error) {
	bySnap := map[int64]int{}
	cache := contentCacheOf(s.Path)
	// a snapshot is known by its ID and its scan: an ID freed by gc may come back later
	all := snapshotIDs(snapOf)
	scanOf, err := s.snapshotScans(all)
	if err != nil {
		return nil, err
	}
	var ids []any
	cache.mu.Lock()
	for _, x := range all {
		id := x.(int64)
		sc, known := scanOf[id]
		if k, ok := cache.kinds[snapKey{id, sc}]; known && ok {
			bySnap[id] = k
		} else {
			ids = append(ids, x)
		}
	}
	cache.mu.Unlock()
	asked := len(ids)
	queried := ids
	// Bound parameters stay below SQLite's limit, even for a full inventory.
	for len(ids) > 0 {
		n := min(len(ids), 500)
		rows, err := s.DB.Query(`SELECT snapshot_id,
		  MAX(rel_path LIKE '%.dll'),
		  MAX((rel_path LIKE 'defs/%' OR rel_path LIKE '%/defs/%' OR rel_path LIKE 'patches/%' OR rel_path LIKE '%/patches/%') AND rel_path LIKE '%.xml'),
		  MAX(rel_path LIKE 'languages/%' OR rel_path LIKE '%/languages/%')
		FROM snapshot_file WHERE snapshot_id IN (`+strings.TrimSuffix(strings.Repeat("?,", n), ",")+`) GROUP BY snapshot_id`, ids[:n]...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var dll, dp, lang int
			if err := rows.Scan(&id, &dll, &dp, &lang); err != nil {
				rows.Close()
				return nil, err
			}
			bySnap[id] = dll*HasDLL | dp*HasDefsPatch | lang*HasLanguages
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		ids = ids[n:]
	}
	if asked > 0 {
		cache.mu.Lock()
		for _, x := range queried {
			id := x.(int64)
			if sc, known := scanOf[id]; known { // no snapshot row: not kept
				cache.kinds[snapKey{id, sc}] = bySnap[id] // no files: 0
			}
		}
		cache.mu.Unlock()
	}
	out := make(map[string]int, len(snapOf))
	for inst, snap := range snapOf {
		out[inst] = bySnap[snap]
	}
	return out, nil
}

// contentCache: snapshot ID → content flags, per database file (read sessions open their own
// Store on the same file).
type contentCache struct {
	mu    sync.Mutex
	kinds map[snapKey]int
}

type snapKey struct{ id, scan int64 }

var contentCaches sync.Map // path → *contentCache

func contentCacheOf(path string) *contentCache {
	c, _ := contentCaches.LoadOrStore(path, &contentCache{kinds: map[snapKey]int{}})
	return c.(*contentCache)
}

// snapshotScans: the scan each snapshot was made in (snapshot ID → scan ID), by primary key.
func (s *Store) snapshotScans(ids []any) (map[int64]int64, error) {
	out := make(map[int64]int64, len(ids))
	for len(ids) > 0 {
		n := min(len(ids), 500)
		rows, err := s.DB.Query(`SELECT id, scan_id FROM snapshot WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", n), ",")+`)`, ids[:n]...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, sc int64
			if err := rows.Scan(&id, &sc); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = sc
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		ids = ids[n:]
	}
	return out, nil
}

// snapshotIDs deduplicates the nonempty current snapshots before querying file rows.
func snapshotIDs(snapOf map[string]int64) []any {
	seen := map[int64]bool{}
	var ids []any
	for _, id := range snapOf {
		if id != 0 && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids
}

// SnapshotsWithPNGFor checks only the requested copies, using snapshot_file's primary key.
func (s *Store) SnapshotsWithPNGFor(snapOf map[string]int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	for _, id := range snapshotIDs(snapOf) {
		var png bool
		err := s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM snapshot_file WHERE snapshot_id=? AND rel_path LIKE '%textures/%.png')`, id).Scan(&png)
		if err != nil {
			return nil, err
		}
		if png {
			out[id.(int64)] = true
		}
	}
	return out, nil
}
