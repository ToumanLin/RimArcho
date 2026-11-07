package gui

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestFileSearchReplacementWaitsForCancelledWorker(t *testing.T) {
	t.Parallel()
	e, s, _ := fixture(t)
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", "Assembly-CSharp.dll"), "synthetic game code")
	started := make(chan int32, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	e.App.Decompile = func(ctx context.Context, _, out, _ string, _ []string) error {
		n := calls.Add(1)
		started <- n
		if n == 1 {
			<-ctx.Done()
			<-release
			return ctx.Err()
		}
		return os.WriteFile(filepath.Join(out, "Game.cs"), []byte("// needle"), 0600)
	}
	q := view.FileSearchReq{Pattern: "needle", Files: "C#", ModsQuery: "id:ludeon.rimworld"}
	first := s.BeginFileSearch()
	if _, err := s.StartFileSearch(q, first); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first worker did not start")
	}
	second := s.BeginFileSearch()
	if _, err := s.StartFileSearch(q, second); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FileSearchUpdate(first, 0); err == nil {
		t.Fatal("old request still accepted")
	}
	select {
	case <-started:
		t.Fatal("replacement overlapped cancelled decompiler")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement did not start")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		u, err := s.FileSearchUpdate(second, 0)
		if err != nil {
			t.Fatal(err)
		}
		if u.Done {
			if u.Error != "" || u.Matches != 1 {
				t.Fatalf("replacement %+v", u)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("replacement did not finish")
}
