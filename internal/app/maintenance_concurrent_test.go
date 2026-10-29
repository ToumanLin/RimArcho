package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestMaintenanceLeaseAcrossConnections(t *testing.T) {
	e := testkit.New(t)
	testkit.MaintenanceHistory(t, e.App.Store, "history", 20, 1000, 220)
	entered, release := make(chan struct{}), make(chan struct{})
	e.App.FreeSpace = func(string) int64 { close(entered); <-release; return 1 << 40 }
	done := make(chan error, 1)
	go func() { _, err := e.App.Maintain(context.Background(), true, false); done <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("maintenance did not reach vacuum")
	}
	// The pruning transaction has committed; only the durable lease now protects
	// the VACUUM gap. Another GUI must not begin a second pass in that gap.
	_, err := e.App.Maintain(context.Background(), true, false)
	close(release)
	if !errors.Is(err, store.ErrMaintenanceBusy) {
		t.Fatalf("concurrent pass: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("maintenance did not finish")
	}
	e.App.FreeSpace = nil
	r, err := e.App.Maintain(context.Background(), true, false)
	if err != nil || !r.Skipped {
		t.Fatalf("completed interval: %+v %v", r, err)
	}
}
