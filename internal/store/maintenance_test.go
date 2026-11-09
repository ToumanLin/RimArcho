package store_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A lease left by a process that is gone (killed during VACUUM) does not block the next
// maintenance; a lease of a running process does, until it expires.
func TestMaintenanceLeaseOfDeadProcess(t *testing.T) {
	e := testkit.New(t)
	s := e.App.Store
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	gone := exec.Command(exe, "-test.run=^$")
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, c := range []struct {
		pid  int
		busy bool
	}{{os.Getpid(), true}, {gone.Process.Pid, false}} {
		lease := now.Add(-time.Minute).UTC().Format(time.RFC3339) + " " + strconv.Itoa(c.pid)
		if _, err := s.DB.Exec(`INSERT INTO meta(key,value) VALUES('maintenance:lease',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, lease); err != nil {
			t.Fatal(err)
		}
		_, err := s.Maintain(context.Background(), store.MaintenanceOptions{Now: now, DryRun: true})
		if busy := errors.Is(err, store.ErrMaintenanceBusy); busy != c.busy || !busy && err != nil {
			t.Fatalf("pid %d (busy %v): %v", c.pid, c.busy, err)
		}
	}
}

// Automatic maintenance without the free space VACUUM needs skips reclamation and still
// completes, so it waits the interval instead of pruning again at every start.
func TestAutomaticMaintenanceShortOfSpace(t *testing.T) {
	e := testkit.New(t)
	s := e.App.Store
	testkit.MaintenanceHistory(t, s, "history", 20, 1000, 220)
	now := time.Now()
	r, err := s.Maintain(context.Background(), store.MaintenanceOptions{Now: now, Automatic: true, FreeSpace: func(string) int64 { return 0 }})
	if err != nil || r.Vacuumed || r.Files == 0 {
		t.Fatalf("%+v %v", r, err)
	}
	if s.Meta(store.MaintenanceLastKey) == "" {
		t.Fatal("not marked complete")
	}
	r, err = s.Maintain(context.Background(), store.MaintenanceOptions{Now: now.Add(time.Hour), Automatic: true, FreeSpace: func(string) int64 { return 0 }})
	if err != nil || !r.Skipped {
		t.Fatalf("second run %+v %v", r, err)
	}
}
