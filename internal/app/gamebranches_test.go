package app

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitBranchFlight(t *testing.T, c *branchCache, key branchKey) {
	t.Helper()
	c.mu.Lock()
	done := c.entries[key].flight
	c.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("branch refresh did not finish")
	}
}

func TestBranchCacheTTLAndStaleSingleflight(t *testing.T) {
	var c branchCache
	key := branchKey{appID: "294100", path: "tool"}
	var clock atomic.Int64
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	var calls atomic.Int32
	load := func() ([]GameBranch, error) { calls.Add(1); return []GameBranch{{Name: "public", BuildID: "old"}}, nil }
	bs, pending, err := c.get(key, now, load)
	if err != nil || pending || bs[0].BuildID != "old" {
		t.Fatalf("first: %v %v %v", bs, pending, err)
	}
	bs[0].BuildID = "mutated"
	clock.Store(int64(gameBranchesTTL - time.Nanosecond))
	bs, pending, err = c.get(key, now, load)
	if err != nil || pending || bs[0].BuildID != "old" || calls.Load() != 1 {
		t.Fatalf("TTL hit: %v %v %v calls=%d", bs, pending, err, calls.Load())
	}
	clock.Store(int64(gameBranchesTTL))
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	refresh := func() ([]GameBranch, error) {
		calls.Add(1)
		close(entered)
		<-release
		return []GameBranch{{Name: "public", BuildID: "new"}}, nil
	}
	bs, pending, err = c.get(key, now, refresh)
	if err != nil || !pending || bs[0].BuildID != "old" {
		t.Fatalf("stale: %v %v %v", bs, pending, err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			bs, pending, err := c.get(key, now, refresh)
			if err != nil || !pending || bs[0].BuildID != "old" {
				t.Errorf("concurrent stale: %v %v %v", bs, pending, err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 2 {
		t.Fatalf("refresh calls=%d", calls.Load())
	}
	once.Do(func() { close(release) })
	waitBranchFlight(t, &c, key)
	bs, pending, err = c.get(key, now, load)
	if err != nil || pending || bs[0].BuildID != "new" || calls.Load() != 2 {
		t.Fatalf("refreshed: %v %v %v calls=%d", bs, pending, err, calls.Load())
	}
}

func TestBranchCacheColdSingleflight(t *testing.T) {
	var c branchCache
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	load := func() ([]GameBranch, error) {
		calls.Add(1)
		close(entered)
		<-release
		return []GameBranch{{Name: "public"}}, nil
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			bs, pending, err := c.get(branchKey{}, time.Now, load)
			if err != nil || pending || len(bs) != 1 {
				t.Errorf("cold: %v %v %v", bs, pending, err)
			}
		})
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("cold query did not start")
	}
	once.Do(func() { close(release) })
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("cold calls=%d", calls.Load())
	}
}

func TestBranchCacheFailureKeepsStale(t *testing.T) {
	var c branchCache
	key := branchKey{}
	clock := time.Unix(0, 0)
	now := func() time.Time { return clock }
	load := func() ([]GameBranch, error) { return []GameBranch{{Name: "public", BuildID: "old"}}, nil }
	c.get(key, now, load)
	clock = clock.Add(gameBranchesTTL)
	failure := errors.New("offline")
	var calls atomic.Int32
	fail := func() ([]GameBranch, error) {
		calls.Add(1)
		return []GameBranch{{Name: "public", BuildID: "partial"}}, failure
	}
	bs, pending, err := c.get(key, now, fail)
	if err != nil || !pending || bs[0].BuildID != "old" {
		t.Fatalf("stale: %v %v %v", bs, pending, err)
	}
	waitBranchFlight(t, &c, key)
	for range 10 {
		bs, pending, err = c.get(key, now, fail)
		if !errors.Is(err, failure) || pending || bs[0].BuildID != "old" {
			t.Fatalf("failed refresh: %v %v %v", bs, pending, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("failure retried during backoff: %d", calls.Load())
	}
	clock = clock.Add(gameBranchesRetry)
	c.get(key, now, load)
	waitBranchFlight(t, &c, key)
	bs, pending, err = c.get(key, now, load)
	if err != nil || pending || bs[0].BuildID != "old" {
		t.Fatalf("recovery: %v %v %v", bs, pending, err)
	}
}

func TestBranchCacheColdFailureRetries(t *testing.T) {
	var c branchCache
	failure := errors.New("offline")
	bs, pending, err := c.get(branchKey{}, time.Now, func() ([]GameBranch, error) { return nil, failure })
	if bs != nil || pending || !errors.Is(err, failure) {
		t.Fatalf("cold failure: %v %v %v", bs, pending, err)
	}
	bs, pending, err = c.get(branchKey{}, time.Now, func() ([]GameBranch, error) { return []GameBranch{{Name: "public"}}, nil })
	if err != nil || pending || len(bs) != 1 {
		t.Fatalf("retry: %v %v %v", bs, pending, err)
	}
}

func TestBranchToolIdentity(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "steamcmd.exe")
	if err := os.WriteFile(exe, []byte("tool"), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := branchToolKey(exe, "294100")
	if err != nil {
		t.Fatal(err)
	}
	same, err := branchToolKey(filepath.Join(dir, ".", "steamcmd.exe"), "294100")
	if err != nil || same != key {
		t.Fatalf("same tool: %v %v", same, err)
	}
	other, _ := branchToolKey(exe, "other-app")
	if other == key {
		t.Fatal("app ID was not part of cache key")
	}
	if err := os.Chtimes(exe, time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	replaced, _ := branchToolKey(exe, "294100")
	if replaced == key {
		t.Fatal("tool modification did not invalidate cache")
	}
	if err := os.WriteFile(exe, []byte("larger tool"), 0600); err != nil {
		t.Fatal(err)
	}
	resized, _ := branchToolKey(exe, "294100")
	if resized == replaced {
		t.Fatal("tool size did not invalidate cache")
	}
	otherExe := filepath.Join(dir, "other.exe")
	if err := os.WriteFile(otherExe, []byte("tool"), 0600); err != nil {
		t.Fatal(err)
	}
	other, _ = branchToolKey(otherExe, "294100")
	if other == key {
		t.Fatal("tool path was not part of cache key")
	}
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if _, err := branchToolKey(exe, "294100"); !os.IsNotExist(err) {
		t.Fatalf("missing tool: %v", err)
	}
	var c branchCache
	calls := 0
	for _, k := range []branchKey{key, same, other, replaced, resized} {
		c.get(k, time.Now, func() ([]GameBranch, error) { calls++; return []GameBranch{{Name: "public"}}, nil })
	}
	if calls != 4 {
		t.Fatalf("identity isolation calls=%d", calls)
	}
}
