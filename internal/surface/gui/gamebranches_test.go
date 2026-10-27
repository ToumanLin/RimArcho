package gui

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// TestGuideDownloadInfoReal measures backend readiness with an isolated, preinstalled
// SteamCMD in $RIMARCHO_BRANCH_BENCH/steamcmd. It never downloads a game.
func TestGuideDownloadInfoReal(t *testing.T) {
	root := os.Getenv("RIMARCHO_BRANCH_BENCH")
	if root == "" {
		t.Skip("opt-in anonymous SteamCMD measurement")
	}
	a, err := app.Open(filepath.Join(root, "home"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Store.SetMeta("cfg:steamcmd.path", filepath.Join(root, "steamcmd", "steamcmd.exe")); err != nil {
		t.Fatal(err)
	}
	s := newAPI(a)
	defer s.Close()
	for i := 0; i < 6; i++ {
		start := time.Now()
		d := s.GuideDownloadInfo()
		elapsed := time.Since(start)
		if d.Error != "" || len(d.Branches) == 0 {
			t.Fatalf("query failed: %s branches=%d", d.Error, len(d.Branches))
		}
		t.Logf("GuideDownloadInfo run=%d elapsed=%.3fms branches=%d", i+1, float64(elapsed.Nanoseconds())/1e6, len(d.Branches))
	}
}

func TestGuideDownloadInfoSingleflight(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.App.SteamCMDDir(), "steamcmd.exe"), "fake")
	e.App.SteamCMDNoPath = true
	out, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "steamcmd", "app_info_print_294100.txt"))
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var calls atomic.Int32
	e.App.SteamCMDRun = func(string, []string) (string, error) {
		calls.Add(1)
		close(entered)
		<-release
		return string(out), nil
	}
	s := newAPI(e.App)
	defer s.Close()
	done := make(chan *DownloadInfo, 16)
	go func() { done <- s.GuideDownloadInfo() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("query did not start")
	}
	// Network work must not hold the GUI mutex.
	unlocked := make(chan struct{})
	go func() { s.mu.Lock(); s.mu.Unlock(); close(unlocked) }()
	select {
	case <-unlocked:
	case <-time.After(time.Second):
		t.Fatal("query held GUI lock")
	}
	for range 15 {
		go func() { done <- s.GuideDownloadInfo() }()
	}
	once.Do(func() { close(release) })
	for range 16 {
		select {
		case d := <-done:
			if d.Error != "" || d.Refreshing || len(d.Branches) == 0 {
				t.Errorf("info: %+v", d)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("query did not finish")
		}
	}
	if d := s.GuideDownloadInfo(); d.Error != "" || len(d.Branches) == 0 || calls.Load() != 1 {
		t.Fatalf("hot info: %+v calls=%d", d, calls.Load())
	}
}
