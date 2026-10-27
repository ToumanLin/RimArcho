package gui

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestBarFilterReusesWorkspaceRowsAndInvalidates(t *testing.T) {
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
