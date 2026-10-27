package gui

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestPageReadersRetainIndexAcrossEditorCommits(t *testing.T) {
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
	readers := []*logReader{&api.logRead, &api.plansRead, &api.libRead, &api.workspaceRead}
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
