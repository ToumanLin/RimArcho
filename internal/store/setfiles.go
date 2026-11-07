package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
)

// Fact rows name their file by an index into their set's file list, fact_set.files: a JSON
// array of paths relative to the mod, in the order the set's rows first use them (05 §3,
// schema v28). A set is written once and never changes, so its list lives and dies with it.

// fileIndex builds a set's file list while its rows are written.
type fileIndex struct {
	ix    map[string]int
	files []string
}

func (x *fileIndex) of(file string) int {
	if i, ok := x.ix[file]; ok {
		return i
	}
	if x.ix == nil {
		x.ix = map[string]int{}
	}
	x.ix[file] = len(x.files)
	x.files = append(x.files, file)
	return len(x.files) - 1
}

func (x *fileIndex) json() string { return encodeFiles(x.files) }

func encodeFiles(files []string) string {
	if len(files) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(files)
	return string(b)
}

// setFileCache keeps file lists by set ID between reads. A deleted set's ID can be given to a
// new set, so the cache is emptied whenever the database changed (data_version and this
// connection's total_changes, as ChangeKey), and when it holds more than setFilesMax paths.
type setFileCache struct {
	mu   sync.Mutex
	key  changeKey
	sets map[int64][]string
	n    int
	ck   *sql.Stmt // the change key query, prepared once (a pragma is slow to parse)
}

const setFilesMax = 1 << 16

type changeKey struct{ dataVersion, totalChanges int64 }

// changeKey is ChangeKey for the cache; the zero key means unknown and never matches.
func (c *setFileCache) changeKey(db *sql.DB) changeKey {
	c.mu.Lock()
	st := c.ck
	if st == nil {
		st, _ = db.Prepare(`SELECT (SELECT data_version FROM pragma_data_version), total_changes()`)
		c.ck = st
	}
	c.mu.Unlock()
	var k changeKey
	if st == nil || st.QueryRow().Scan(&k.dataVersion, &k.totalChanges) != nil {
		return changeKey{}
	}
	return k
}

func (c *setFileCache) close() {
	c.mu.Lock()
	if c.ck != nil {
		c.ck.Close()
		c.ck = nil
	}
	c.mu.Unlock()
}

// fileLists returns the file lists of the given sets.
func (s *Store) fileLists(ids map[int64]bool) (map[int64][]string, error) {
	out := make(map[int64][]string, len(ids))
	var miss []any
	c, key := s.setFiles, changeKey{}
	if c != nil {
		key = c.changeKey(s.DB)
		c.mu.Lock()
		if c.key != key || key == (changeKey{}) {
			c.key, c.sets, c.n = key, map[int64][]string{}, 0
		}
		for id := range ids {
			if l, ok := c.sets[id]; ok {
				out[id] = l
			} else {
				miss = append(miss, id)
			}
		}
		c.mu.Unlock()
	} else {
		for id := range ids {
			miss = append(miss, id)
		}
	}
	read := map[int64][]string{}
	for len(miss) > 0 {
		n := min(len(miss), 500)
		rows, err := s.DB.Query(`SELECT id, files FROM fact_set WHERE id IN (`+placeholders(n)+`)`, miss[:n]...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var data string
			if err := rows.Scan(&id, &data); err != nil {
				rows.Close()
				return nil, err
			}
			l, err := decodeFiles(id, data)
			if err != nil {
				rows.Close()
				return nil, err
			}
			out[id], read[id] = l, l
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		miss = miss[n:]
	}
	if c != nil && len(read) > 0 {
		c.mu.Lock()
		if c.key == key && key != (changeKey{}) {
			for id, l := range read {
				// Full: drop sets at random (map order) down to half, so a pass over more
				// sets than fit still hits for some instead of emptying the cache each time.
				if c.n+len(l) > setFilesMax {
					for old, ol := range c.sets {
						delete(c.sets, old)
						c.n -= len(ol)
						if c.n+len(l) <= setFilesMax/2 {
							break
						}
					}
				}
				c.sets[id] = l
				c.n += len(l)
			}
		}
		c.mu.Unlock()
	}
	return out, nil
}

func decodeFiles(set int64, data string) ([]string, error) {
	var l []string
	if err := json.Unmarshal([]byte(data), &l); err != nil {
		return nil, fmt.Errorf("store: file list of fact set %d: %w", set, err)
	}
	return l, nil
}

// fileRef is a fact row's (set, index) while it waits for its path.
type fileRef struct {
	set int64
	ix  int
	to  *string
}

// resolveFiles fills each reference's path from its set's list.
func (s *Store) resolveFiles(refs []fileRef) error {
	if len(refs) == 0 {
		return nil
	}
	ids := map[int64]bool{}
	for _, r := range refs {
		ids[r.set] = true
	}
	lists, err := s.fileLists(ids)
	if err != nil {
		return err
	}
	for _, r := range refs {
		l := lists[r.set]
		if r.ix < 0 || r.ix >= len(l) {
			return fmt.Errorf("store: fact set %d has no file %d", r.set, r.ix)
		}
		*r.to = l[r.ix]
	}
	return nil
}
