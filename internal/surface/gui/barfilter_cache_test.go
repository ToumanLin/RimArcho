package gui

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestBarFilterReusesWorkspaceRowsAndInvalidates(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	api.Workspace(1)
	first := environmentResult(t, events, 1)
	if first.Error != "" || first.Stale {
		t.Fatalf("workspace: %+v", first)
	}
	rows := api.workspaceReader.board
	check := func(wantActive int) {
		t.Helper()
		result, err := api.BarFilter("", nil, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Rows[1]) != wantActive {
			t.Fatalf("active rows: %d, want %d", len(result.Rows[1]), wantActive)
		}
	}
	check(len(first.Board.Cols[view.Active]))
	if api.workspaceReader.board != rows {
		t.Fatal("filter rebuilt unchanged workspace rows")
	}
	id := first.Board.Cols[view.Active][1].ID
	if _, err := api.Toggle(int(view.Active), []string{id}, nil); err != nil {
		t.Fatal(err)
	}
	check(len(first.Board.Cols[view.Active]) - 1)
	if api.workspaceReader.board == rows {
		t.Fatal("filter reused rows from before list edit")
	}
	rows = api.workspaceReader.board
	lineage := rows.St.Ix.ByID[id].LineageID
	if err := e.App.Store.SetAlias("", lineage, "F4 renamed"); err != nil {
		t.Fatal(err)
	}
	result, err := api.BarFilter("F4 renamed", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if api.workspaceReader.board == rows || len(result.Rows[0]) != 1 {
		t.Fatalf("filter kept stale index: %+v", result.Rows)
	}
}

func TestBarFilterRefreshesOtherListMembership(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	api.Workspace(1)
	first := environmentResult(t, events, 1)
	if first.Error != "" || first.Stale {
		t.Fatalf("workspace: %+v", first)
	}
	e.MustRun("plan", "new", "Other colony", "--from", "current")
	result, err := api.BarFilter(`lists:"Other colony"`, nil, true)
	if err != nil || len(result.Rows[1]) != len(first.Board.Cols[1]) {
		t.Fatalf("new list membership: %+v, %v", result, err)
	}
	other, err := e.App.Store.PlanByName("Other colony")
	if err != nil || other == nil {
		t.Fatalf("other list: %v", err)
	}
	e.App.OpenPlan = other.ID
	e.MustRun("disable", first.Board.Cols[1][1].Ref)
	e.App.OpenPlan = 0
	result, err = api.BarFilter(`lists:"Other colony"`, nil, true)
	if err != nil || len(result.Rows[1]) != len(first.Board.Cols[1])-1 {
		t.Fatalf("changed membership stayed cached: %+v, %v", result, err)
	}
	e.MustRun("plan", "rename", "Other colony", "Renamed colony")
	result, err = api.BarFilter(`lists:"Renamed colony"`, nil, true)
	if err != nil || len(result.Rows[1]) != len(first.Board.Cols[1])-1 {
		t.Fatalf("renamed list stayed cached: %+v, %v", result, err)
	}
}
