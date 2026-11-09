// Package machine reads the machine RimArcho runs on without scanning game or mod files:
// hardware capacity, Wine, and the console.
package machine

import (
	"sync"
	"time"
)

type Adapter struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Dedicated int64  `json:"dedicated"`
}

type Memory struct {
	TotalRAM        int64     `json:"totalRAM"`
	AvailableRAM    int64     `json:"availableRAM"`
	CommitAvailable int64     `json:"commitAvailable"`
	Adapters        []Adapter `json:"adapters,omitempty"`
	SampledAt       time.Time `json:"sampledAt"`
	Error           string    `json:"error,omitempty"`
}

// VRAM uses Windows' high-performance adapter ordering. This is a candidate,
// not proof of the adapter selected by the game's renderer or per-app overrides.
func (m Memory) VRAM() int64 {
	if len(m.Adapters) == 0 {
		return 0
	}
	return m.Adapters[0].Dedicated
}

var cache struct {
	sync.Mutex
	value      Memory
	refreshing bool
	done       chan struct{}
	checked    time.Time
}

// Cached never waits for a native probe. The first call starts one background read;
// further reads reuse the snapshot for 30 seconds, including failed probes.
func Cached() Memory {
	cache.Lock()
	defer cache.Unlock()
	if !cache.refreshing && time.Since(cache.checked) >= 30*time.Second {
		cache.refreshing = true
		cache.done = make(chan struct{})
		go func() {
			m := Read()
			cache.Lock()
			cache.value = m
			cache.checked = time.Now()
			cache.refreshing = false
			close(cache.done)
			cache.Unlock()
		}()
	}
	return cache.value
}

// Measure refreshes a missing snapshot in a caller that already runs off the page path.
func Measure() Memory {
	m := Cached()
	if !m.SampledAt.IsZero() {
		return m
	}
	cache.Lock()
	done := cache.done
	cache.Unlock()
	if done != nil {
		<-done
	}
	cache.Lock()
	defer cache.Unlock()
	return cache.value
}
