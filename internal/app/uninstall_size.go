package app

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
)

const uninstallSizeTTL = time.Minute

// UninstallSizeCache holds only byte estimates, never paths' deletion eligibility.
// Its zero value is ready for concurrent preview requests.
type UninstallSizeCache struct {
	mu      sync.Mutex
	entries map[string]uninstallSize
}

type uninstallSize struct {
	bytes int64
	until time.Time
}

func (c *UninstallSizeCache) sizeOf(dir string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	path, err := filepath.Abs(dir)
	if err != nil {
		path = filepath.Clean(dir)
	}
	key, now := env.PathKey(path), time.Now()
	if v, ok := c.entries[key]; ok && now.Before(v.until) {
		return v.bytes
	}
	// Expiry also bounds retention when configured directories change.
	for k, v := range c.entries {
		if !now.Before(v.until) {
			delete(c.entries, k)
		}
	}
	n := dirSize(path)
	if c.entries == nil {
		c.entries = make(map[string]uninstallSize)
	}
	c.entries[key] = uninstallSize{bytes: n, until: time.Now().Add(uninstallSizeTTL)}
	return n
}
