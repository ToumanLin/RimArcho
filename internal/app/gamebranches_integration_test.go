package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestGameBranchesRunnerAndInstanceLock(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	exe := filepath.Join(e.App.SteamCMDDir(), deps.SteamCMDExecutable())
	e.WriteFile(exe, "fake")
	e.App.SteamCMDNoPath = true
	out, err := os.ReadFile(filepath.Join("..", "..", "testdata", "steamcmd", "app_info_print_294100.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	entered := make(chan struct{}, 1)
	e.App.SteamCMDRun = func(path string, args []string) (string, error) {
		calls.Add(1)
		entered <- struct{}{}
		want := []string{"+login", "anonymous", "+app_info_update", "1", "+app_info_print", "294100", "+quit"}
		if path != exe || !slices.Equal(args, want) {
			t.Errorf("runner %s %v", path, args)
		}
		return string(out), nil
	}
	unlock := accounts.Lock(exe)
	var once sync.Once
	defer once.Do(unlock)
	done := make(chan struct{})
	go func() {
		defer close(done)
		bs, pending, err := e.App.GameBranchesCached(exe)
		if err != nil || pending || len(bs) == 0 {
			t.Errorf("query %v %v %v", bs, pending, err)
		}
	}()
	select {
	case <-entered:
		t.Fatal("runner bypassed instance lock")
	case <-time.After(30 * time.Millisecond):
	}
	once.Do(unlock)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("query blocked")
	}
	bs, err := e.App.GameBranches()
	if err != nil || len(bs) == 0 || calls.Load() != 1 {
		t.Fatalf("hot query: %v %v calls=%d", bs, err, calls.Load())
	}
	// The cache is shared by application read sessions, not tied to an App instance.
	reader, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	bs, pending, err := reader.GameBranchesCached(exe)
	if err != nil || pending || len(bs) == 0 || calls.Load() != 1 {
		t.Fatalf("shared query: %v %v %v calls=%d", bs, pending, err, calls.Load())
	}
	// Replacing the tool invalidates even a fresh entry. Partial output on a failed
	// process must not become a successful cached result.
	e.WriteFile(exe, "replaced fake")
	failure := errors.New("process failed")
	e.App.SteamCMDRun = func(string, []string) (string, error) { calls.Add(1); return string(out), failure }
	if _, _, err := e.App.GameBranchesCached(exe); err == nil {
		t.Fatal("partial failed output was cached")
	}
	e.App.SteamCMDRun = func(string, []string) (string, error) { calls.Add(1); return "not app info", nil }
	if _, _, err := e.App.GameBranchesCached(exe); err == nil {
		t.Fatal("malformed output was cached")
	}
	e.App.SteamCMDRun = func(string, []string) (string, error) { calls.Add(1); return string(out), nil }
	if bs, _, err := e.App.GameBranchesCached(exe); err != nil || len(bs) == 0 {
		t.Fatalf("retry: %v %v", bs, err)
	}
	if calls.Load() != 4 {
		t.Fatalf("calls=%d", calls.Load())
	}
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.GameBranches(); !errors.Is(err, app.ErrNoSteamCMD) {
		t.Fatalf("removed tool: %v", err)
	}
}
