package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/harmony"
)

// The Harmony fact cache of checks (analyze.AssemblyCache, 05 §5.3): one JSON file per DLL
// content and skip flag in <data>/cache/harmony/. A file in use gets a new time at most once
// per asmCacheTouch; after each check, files that nothing used for asmCacheUnused go.
const (
	asmCacheUnused = 30 * 24 * time.Hour
	asmCacheTouch  = 7 * 24 * time.Hour
)

type asmCache struct {
	dir  string
	mu   sync.Mutex
	used map[string]bool
}

func (a *App) assemblyCache() *asmCache {
	if a.Home == "" {
		return nil
	}
	return &asmCache{dir: a.cacheDir("harmony"), used: map[string]bool{}}
}

func (c *asmCache) file(key string) string { return filepath.Join(c.dir, key+".json") }

func (c *asmCache) note(key string) {
	c.mu.Lock()
	c.used[key+".json"] = true
	c.mu.Unlock()
}

// Get reads the facts stored under key.
func (c *asmCache) Get(key string) (*harmony.Assembly, bool) {
	c.note(key)
	p := c.file(key)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	var asm harmony.Assembly
	if json.Unmarshal(b, &asm) != nil {
		return nil, false
	}
	if fi, err := os.Stat(p); err == nil && time.Since(fi.ModTime()) > asmCacheTouch {
		now := time.Now()
		os.Chtimes(p, now, now)
	}
	return &asm, true
}

// Put stores the facts of a DLL; a failed write only costs the next check a read.
func (c *asmCache) Put(key string, asm *harmony.Assembly) {
	c.note(key)
	b, err := json.Marshal(asm)
	if err != nil || os.MkdirAll(c.dir, 0o755) != nil {
		return
	}
	tmp, err := os.CreateTemp(c.dir, "*.tmp")
	if err != nil {
		return
	}
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), c.file(key))
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
}

// prune removes the files this check did not use that nothing used for asmCacheUnused, and
// temporary files left by a check that stopped (an hour old).
func (c *asmCache) prune() {
	if c == nil {
		return
	}
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if c.used[e.Name()] {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		age := time.Since(fi.ModTime())
		if age > asmCacheUnused || strings.HasSuffix(e.Name(), ".tmp") && age > time.Hour {
			os.Remove(filepath.Join(c.dir, e.Name()))
		}
	}
}
