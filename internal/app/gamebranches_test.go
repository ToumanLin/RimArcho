package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
)

func waitBranchFlight(t *testing.T, c *branchCache, key branchKey) {
	t.Helper()
	c.mu.Lock()
	done := c.entries[key.cacheKey()].flight
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
	bs, pending, err := c.get(key, now, load, nil)
	if err != nil || pending || bs[0].BuildID != "old" {
		t.Fatalf("first: %v %v %v", bs, pending, err)
	}
	bs[0].BuildID = "mutated"
	clock.Store(int64(gameBranchesTTL - time.Nanosecond))
	bs, pending, err = c.get(key, now, load, nil)
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
	bs, pending, err = c.get(key, now, refresh, nil)
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
			bs, pending, err := c.get(key, now, refresh, nil)
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
	bs, pending, err = c.get(key, now, load, nil)
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
			bs, pending, err := c.get(branchKey{}, time.Now, load, nil)
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
	c.get(key, now, load, nil)
	clock = clock.Add(gameBranchesTTL)
	failure := errors.New("offline")
	var calls atomic.Int32
	fail := func() ([]GameBranch, error) {
		calls.Add(1)
		return []GameBranch{{Name: "public", BuildID: "partial"}}, failure
	}
	bs, pending, err := c.get(key, now, fail, nil)
	if err != nil || !pending || bs[0].BuildID != "old" {
		t.Fatalf("stale: %v %v %v", bs, pending, err)
	}
	waitBranchFlight(t, &c, key)
	for range 10 {
		bs, pending, err = c.get(key, now, fail, nil)
		if !errors.Is(err, failure) || pending || bs[0].BuildID != "old" {
			t.Fatalf("failed refresh: %v %v %v", bs, pending, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("failure retried during backoff: %d", calls.Load())
	}
	clock = clock.Add(gameBranchesRetry)
	c.get(key, now, load, nil)
	waitBranchFlight(t, &c, key)
	bs, pending, err = c.get(key, now, load, nil)
	if err != nil || pending || bs[0].BuildID != "old" {
		t.Fatalf("recovery: %v %v %v", bs, pending, err)
	}
}

func TestBranchCacheColdFailureRetries(t *testing.T) {
	var c branchCache
	failure := errors.New("offline")
	bs, pending, err := c.get(branchKey{}, time.Now, func() ([]GameBranch, error) { return nil, failure }, nil)
	if bs != nil || pending || !errors.Is(err, failure) {
		t.Fatalf("cold failure: %v %v %v", bs, pending, err)
	}
	bs, pending, err = c.get(branchKey{}, time.Now, func() ([]GameBranch, error) { return []GameBranch{{Name: "public"}}, nil }, nil)
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
		c.get(k, time.Now, func() ([]GameBranch, error) { calls++; return []GameBranch{{Name: "public"}}, nil }, nil)
	}
	if calls != 4 {
		t.Fatalf("identity isolation calls=%d", calls)
	}
}

func TestGameBranchesSelfUpdateRetainsCache(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "steamcmd.exe")
	if err := os.WriteFile(exe, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	a := &App{SteamCMDRun: func(string, []string) (string, error) {
		calls.Add(1)
		if err := os.WriteFile(exe, []byte("updated executable"), 0600); err != nil {
			return "", err
		}
		return `"294100" { "depots" { "branches" { "public" { "buildid" "2" } } } }`, nil
	}}
	for range 2 {
		bs, pending, err := a.GameBranchesCached(exe)
		if err != nil || pending || len(bs) != 1 || bs[0].BuildID != "2" {
			t.Fatalf("branches: %v %v %v", bs, pending, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("self-update caused another query: %d", calls.Load())
	}
}

func TestBranchCacheSelfUpdateSingleflight(t *testing.T) {
	var c branchCache
	old := branchKey{path: "self-update", size: 1}
	updated := old
	updated.size = 2
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	result := make(chan error, 2)
	go func() {
		_, _, err := c.get(old, time.Now, func() ([]GameBranch, error) {
			close(entered)
			<-release
			return []GameBranch{{Name: "public"}}, nil
		}, func() (branchKey, error) { return updated, nil })
		result <- err
	}()
	<-entered
	go func() {
		_, _, err := c.get(updated, time.Now, func() ([]GameBranch, error) { return nil, errors.New("duplicate query during self-update") }, nil)
		result <- err
	}()
	once.Do(func() { close(release) })
	for range 2 {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("query did not finish")
		}
	}
	bs, pending, err := c.get(updated, time.Now, func() ([]GameBranch, error) { t.Error("miss after self-update"); return nil, nil }, nil)
	if err != nil || pending || len(bs) != 1 {
		t.Fatalf("cache: %v %v %v", bs, pending, err)
	}
}

func TestBranchRefreshCanceledWhileWaitingForLock(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "steamcmd.exe")
	if err := os.WriteFile(exe, []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	a := &App{SteamCMDRun: func(string, []string) (string, error) {
		calls.Add(1)
		return `"294100" { "depots" { "branches" { "public" { "buildid" "old" } } } }`, nil
	}}
	old, _, err := a.GameBranchesCached(exe)
	if err != nil {
		t.Fatal(err)
	}
	key, err := branchToolKey(exe, "294100")
	if err != nil {
		t.Fatal(err)
	}
	gameBranchesCache.mu.Lock()
	gameBranchesCache.entries[key.cacheKey()].next = time.Time{}
	gameBranchesCache.mu.Unlock()
	unlock := accounts.Lock(exe)
	var once sync.Once
	defer once.Do(unlock)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bs, pending, err := a.GameBranchesCachedContext(ctx, exe)
	if err != nil || !pending || !slices.Equal(bs, old) {
		t.Fatalf("stale: %v %v %v", bs, pending, err)
	}
	cancel()
	once.Do(unlock)
	waitBranchFlight(t, &gameBranchesCache, key)
	if calls.Load() != 1 {
		t.Fatal("canceled refresh started SteamCMD after waiting for lock")
	}
	gameBranchesCache.mu.Lock()
	entry := gameBranchesCache.entries[key.cacheKey()]
	kept, refreshErr := slices.Clone(entry.branches), entry.err
	gameBranchesCache.mu.Unlock()
	if !slices.Equal(kept, old) || !errors.Is(refreshErr, context.Canceled) {
		t.Fatalf("canceled refresh: %v %v", kept, refreshErr)
	}
	// Cancellation belongs to the closing window; another window can retry now.
	_, pending, err = a.GameBranchesCached(exe)
	if err != nil || !pending {
		t.Fatalf("retry: %v %v", pending, err)
	}
	waitBranchFlight(t, &gameBranchesCache, key)
	if calls.Load() != 2 {
		t.Fatalf("retry calls=%d", calls.Load())
	}
}

// A separate test process provides a real running command without launching SteamCMD.
func TestBranchCommandHelper(t *testing.T) {
	i := slices.Index(os.Args, "--branch-command-helper")
	if i < 0 {
		return
	}
	if err := os.WriteFile(os.Args[i+1], []byte("ready"), 0600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

func TestBranchCommandCanceled(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := runBranchSteamCMD(ctx, exe, []string{"-test.run=^TestBranchCommandHelper$", "--", "--branch-command-helper", ready})
		done <- err
	}()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("helper exited before cancellation: %v", err)
		case <-deadline:
			t.Fatal("helper did not start")
		case <-tick.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled command succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("command kept running after cancellation")
	}
}
