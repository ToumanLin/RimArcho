package app

import (
	"encoding/binary"
	"strings"
	"sync"
	"weak"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/zeebo/xxh3"
)

// Read-only results that every session of this process may share (17 §7.3). The window
// keeps several read sessions on one database; each built its own list registry and
// game-link instances from the same data, about 4 and 5.5 MB per session for 6,574 Mod
// copies. These caches hold weak pointers: an entry lives only while a session uses it,
// so a closed or idle-expired reader does not keep it alive.

// sharedRegistry returns a registry for exactly these candidates, reusing one another
// session built from identical candidates. Registries are never changed after
// NewRegistry; Given is clipped, so a caller that appends gets a copy.
func sharedRegistry(all []loadout.Candidate) *loadout.Registry {
	sum := candidateSum(all)
	registries.Lock()
	if p, ok := registries.m[sum]; ok {
		if r := p.Value(); r != nil {
			registries.Unlock()
			return r
		}
	}
	registries.Unlock()
	r := loadout.NewRegistry(all) // outside the lock: other sessions keep reading
	registries.Lock()
	defer registries.Unlock()
	if p, ok := registries.m[sum]; ok {
		if other := p.Value(); other != nil {
			return other
		}
	}
	for k, p := range registries.m {
		if p.Value() == nil {
			delete(registries.m, k)
		}
	}
	registries.m[sum] = weak.Make(r)
	return r
}

var registries = struct {
	sync.Mutex
	m map[xxh3.Uint128]weak.Pointer[loadout.Registry]
}{m: map[xxh3.Uint128]weak.Pointer[loadout.Registry]{}}

// candidateSum identifies a candidate list by content and order (every field, length
// prefixed, so no two different lists can be confused short of a 128-bit collision).
func candidateSum(all []loadout.Candidate) xxh3.Uint128 {
	h := xxh3.New()
	var n [8]byte
	put := func(s string) {
		binary.LittleEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.WriteString(s)
	}
	binary.LittleEndian.PutUint64(n[:], uint64(len(all)))
	h.Write(n[:])
	for i := range all {
		c := &all[i]
		put(c.ID)
		put(c.PackageID)
		put(string(c.Kind))
		put(c.Folder)
		put(c.Lineage)
	}
	return h.Sum128()
}

// sharedLinks is the game-link instances another session built for this database at
// this index generation, or nil. Only the shared generation counter identifies a state
// across connections ("i<token>.<n>"); a connection's own fallback key does not.
func sharedLinks(s *store.Store, key string) *linkInstances {
	if !strings.HasPrefix(key, "i") {
		return nil
	}
	links.Lock()
	defer links.Unlock()
	if p, ok := links.m[linkKey(s, key)]; ok {
		return p.Value()
	}
	return nil
}

// shareLinks offers li to the other sessions; it is not changed after linkInstances.
func shareLinks(s *store.Store, li *linkInstances) {
	if !strings.HasPrefix(li.key, "i") {
		return
	}
	links.Lock()
	defer links.Unlock()
	for k, p := range links.m {
		if p.Value() == nil {
			delete(links.m, k)
		}
	}
	links.m[linkKey(s, li.key)] = weak.Make(li)
}

// forgetLinks drops this database's shared game-link instances: a change the index
// generation does not count (a moved managed directory) must reach the other sessions.
func forgetLinks(s *store.Store) {
	links.Lock()
	defer links.Unlock()
	for k := range links.m {
		if strings.HasPrefix(k, s.Path+"|") {
			delete(links.m, k)
		}
	}
}

// linkKey names a database and generation ("|" cannot occur in a Windows path).
func linkKey(s *store.Store, key string) string { return s.Path + "|" + key }

var links = struct {
	sync.Mutex
	m map[string]weak.Pointer[linkInstances]
}{m: map[string]weak.Pointer[linkInstances]{}}

// declReads is the read connection that lazy declaration reads of shared index Mods use.
// One per opened data directory: its read sessions share it, it opens on first use and
// closes with the App that opened the directory. Snapshot rows never change their
// declaration, so any connection reads the same one.
type declReads struct {
	mu     sync.Mutex
	path   string
	s      *store.Store
	closed bool
}

// declStore is the declaration connection, or nil (an App not from Open, or closing).
func (a *App) declStore() *store.Store {
	d := a.decl
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.s == nil && !d.closed {
		s, err := store.OpenSession(d.path)
		if err != nil {
			return nil
		}
		s.DB.Exec("PRAGMA cache_size=-256")
		d.s = s
	}
	return d.s
}

func (d *declReads) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	if d.s != nil {
		d.s.Close()
		d.s = nil
	}
}
