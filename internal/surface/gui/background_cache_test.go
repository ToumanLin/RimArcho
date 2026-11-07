package gui

import (
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestPageReadersRetainIndexAcrossEditorCommits(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	b := api.Board()
	id := b.Cols[view.Active][1].ID
	read := func(request uint64) {
		t.Helper()
		if h := api.Home(); h.Error != "" {
			t.Fatal(h.Error)
		}
		if sh := api.Shell(); sh.Error != "" {
			t.Fatal(sh.Error)
		}
		if _, err := api.PlanManage(0); err != nil {
			t.Fatal(err)
		}
		if _, err := api.Library("", nil, "name"); err != nil {
			t.Fatal(err)
		}
		api.Workspace(request)
		if r := environmentResult(t, events, request); r.Error != "" || r.Stale {
			t.Fatalf("workspace: %+v", r)
		}
	}
	read(1)
	readers := []*logReader{&api.logRead, &api.plansRead, &api.libRead, &api.workspaceReader}
	apps := make([]*app.App, len(readers))
	indexes := make([]*app.Index, len(readers))
	for i, r := range readers {
		apps[i] = r.a
		var err error
		indexes[i], err = r.a.Index()
		if err != nil {
			t.Fatal(err)
		}
		if r.a == e.App || indexes[i] == b.St.Ix {
			t.Fatal("page shares mutable editor state")
		}
	}
	for n := uint64(2); n < 5; n++ {
		if _, err := api.Toggle(int(view.Active), []string{id}, nil); err != nil {
			t.Fatal(err)
		}
		read(n)
		for i, r := range readers {
			ix, err := r.a.Index()
			if err != nil || r.a != apps[i] || ix != indexes[i] {
				t.Fatalf("reader %d rebuilt after list edit: %v", i, err)
			}
		}
	}
	// A real index change must still invalidate every retained reader.
	if err := e.App.Store.SetAlias("", indexes[0].ByID[id].LineageID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	read(5)
	for i, r := range readers {
		ix, err := r.a.Index()
		if err != nil || ix == indexes[i] || ix.ByID[id].DisplayName() != "Renamed" {
			t.Fatalf("reader %d kept stale metadata: %v", i, err)
		}
	}
}

func TestWorkspaceInvalidatedIndexLoadsOutsidePageMutex(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	board := api.Board()
	api.Workspace(1)
	first := environmentResult(t, events, 1)
	if first.Error != "" || first.Stale {
		t.Fatalf("workspace: %+v", first)
	}
	id := board.Cols[view.Active][1].ID
	if err := e.App.Store.SetAlias("", board.St.Ix.ByID[id].LineageID, "Invalidated"); err != nil {
		t.Fatal(err)
	}
	started, release := pauseInventory(t, api)
	api.Workspace(2)
	awaitInventory(t, started)
	done := make(chan struct{})
	go func() { api.Init(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second): // the blocker is held until released; this only bounds a hang
		release()
		t.Fatal("invalidated workspace loaded the index while holding the page mutex")
	}
	release()
	if result := environmentResult(t, events, 2); result.Error != "" || result.Stale {
		t.Fatalf("workspace: %+v", result)
	}
}
