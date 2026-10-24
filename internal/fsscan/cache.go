package fsscan

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const maxCacheBytes = 64 << 20

type cachedValue struct {
	Stamp string          `json:"stamp"`
	Value json.RawMessage `json:"value"`
}

type cacheFile struct {
	Version int                               `json:"v"`
	Entries map[string]map[string]cachedValue `json:"entries"`
}

// Cache stores JSON values by key, consumer name and fingerprint.
// Its methods are safe for concurrent use. Use OpenCache to create one.
type Cache struct {
	mu      sync.Mutex
	path    string
	entries map[string]map[string]cachedValue
	dirty   bool
}

// OpenCache loads path. Missing, damaged, unsupported or oversized files
// produce an empty cache. Reads are limited to 64 MiB.
func OpenCache(path string) *Cache {
	c := &Cache{path: path, entries: make(map[string]map[string]cachedValue)}
	f, err := os.Open(path)
	if err != nil {
		return c
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() > maxCacheBytes {
		return c
	}
	data, err := io.ReadAll(io.LimitReader(f, maxCacheBytes))
	if err != nil {
		return c
	}
	var saved cacheFile
	if json.Unmarshal(data, &saved) != nil || saved.Version != 1 {
		return c
	}
	for _, names := range saved.Entries {
		for _, value := range names {
			stamp, err := hex.DecodeString(value.Stamp)
			if err != nil || len(stamp) != 16 || len(value.Value) == 0 {
				return c
			}
		}
	}
	if saved.Entries != nil {
		c.entries = saved.Entries
	}
	return c
}

// Get decodes the value under (key, name) into v when its stamp matches.
// It returns false on a missing value, a stamp mismatch or a decoding error.
func (c *Cache) Get(key, name string, stamp [16]byte, v any) bool {
	c.mu.Lock()
	value, ok := c.entries[key][name]
	c.mu.Unlock()
	return ok && value.Stamp == hex.EncodeToString(stamp[:]) && json.Unmarshal(value.Value, v) == nil
}

// Put marshals v now and stores it under (key, name, stamp).
// Values that cannot be marshaled leave the cache unchanged.
func (c *Cache) Put(key, name string, stamp [16]byte, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	value := cachedValue{hex.EncodeToString(stamp[:]), data}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.entries[key][name]; ok && old.Stamp == value.Stamp && bytes.Equal(old.Value, data) {
		return
	}
	if c.entries[key] == nil {
		c.entries[key] = make(map[string]cachedValue)
	}
	c.entries[key][name] = value
	c.dirty = true
}

// Prune removes keys for which keep returns false.
// keep runs with the cache locked and must not call its methods.
func (c *Cache) Prune(keep func(key string) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if !keep(key) {
			delete(c.entries, key)
			c.dirty = true
		}
	}
}

// Save writes changes through a temporary file and rename in the same directory.
// It creates parent directories and does no I/O when the cache is unchanged.
func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty {
		return nil
	}
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(c.path)+"-*")
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()
	if err = json.NewEncoder(f).Encode(cacheFile{1, c.entries}); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), c.path); err != nil {
		return err
	}
	c.dirty = false
	return nil
}
