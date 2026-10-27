package app

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

const gameBranchesTTL = 10 * time.Minute
const gameBranchesRetry = 30 * time.Second

// Shared across read sessions; tool replacement must not reuse another binary's data.
var gameBranchesCache branchCache

type branchKey struct {
	appID, path    string
	size, modified int64
}

func branchToolKey(exe, appID string) (branchKey, error) {
	path, err := filepath.Abs(exe)
	if err != nil {
		return branchKey{}, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return branchKey{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return branchKey{}, err
	}
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return branchKey{appID: appID, path: path, size: info.Size(), modified: info.ModTime().UnixNano()}, nil
}

type branchCache struct {
	mu      sync.Mutex
	entries map[branchKey]*branchEntry
}

type branchEntry struct {
	branches []GameBranch
	next     time.Time
	flight   chan struct{}
	err      error
}

func (c *branchCache) get(key branchKey, now func() time.Time, load func() ([]GameBranch, error)) ([]GameBranch, bool, error) {
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[branchKey]*branchEntry)
	}
	e := c.entries[key]
	if e == nil {
		e = &branchEntry{}
		c.entries[key] = e
	}
	if len(e.branches) > 0 && (now().Before(e.next) || e.flight != nil) {
		bs, pending, err := slices.Clone(e.branches), e.flight != nil, e.err
		c.mu.Unlock()
		return bs, pending, err
	}
	if e.flight != nil {
		done := e.flight
		c.mu.Unlock()
		<-done
		c.mu.Lock()
		bs, err := slices.Clone(e.branches), e.err
		c.mu.Unlock()
		return bs, false, err
	}
	e.flight = make(chan struct{})
	if len(e.branches) > 0 {
		bs := slices.Clone(e.branches)
		c.mu.Unlock()
		go c.refresh(e, now, load)
		return bs, true, nil
	}
	c.mu.Unlock()
	c.refresh(e, now, load)
	c.mu.Lock()
	bs, err := slices.Clone(e.branches), e.err
	c.mu.Unlock()
	return bs, false, err
}

func (c *branchCache) refresh(e *branchEntry, now func() time.Time, load func() ([]GameBranch, error)) {
	bs, err := load()
	c.mu.Lock()
	defer c.mu.Unlock()
	e.err = err
	if err == nil {
		e.branches = slices.Clone(bs)
		e.next = now().Add(gameBranchesTTL)
	} else {
		// Polling a failed refresh must not start SteamCMD repeatedly.
		e.next = now().Add(gameBranchesRetry)
	}
	close(e.flight)
	e.flight = nil
}
