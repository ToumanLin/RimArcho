package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestMaintenanceLeaseAcrossConnections(t *testing.T) {
	e := testkit.New(t)
	testkit.MaintenanceHistory(t, e.App.Store, "history", 20, 1000, 220)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	e.App.FreeSpace = func(string) int64 { close(entered); <-release; return 1 << 40 }
	done := make(chan error, 1)
	go func() { _, err := e.App.Maintain(context.Background(), true, false); done <- err }()
	// Registered after testkit.New, so it runs before the store closes and the TempDir
	// goes: a failed wait must not leave the first pass parked holding archo.db open.
	finished := false
	t.Cleanup(func() {
		unblock()
		if !finished {
			<-done
		}
	})
	// Pruning 20,000 rows takes about 3 s under -race on a busy runner: the bounds only
	// catch a pass that never gets there, they are not timings.
	select {
	case <-entered:
	case <-time.After(time.Minute):
		t.Fatal("maintenance did not reach vacuum")
	}
	// The pruning transaction has committed; only the durable lease now protects
	// the VACUUM gap. Another GUI must not begin a second pass in that gap.
	_, err := e.App.Maintain(context.Background(), true, false)
	unblock()
	if !errors.Is(err, store.ErrMaintenanceBusy) {
		t.Fatalf("concurrent pass: %v", err)
	}
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Minute):
		t.Fatal("maintenance did not finish")
	}
	e.App.FreeSpace = nil
	r, err := e.App.Maintain(context.Background(), true, false)
	if err != nil || !r.Skipped {
		t.Fatalf("completed interval: %+v %v", r, err)
	}
}
