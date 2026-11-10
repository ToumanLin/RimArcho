package view_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// "Directory check failed" is a directory need too: a refresh replaces it instead of adding
// another, and drops it once the check works again.
func TestRefreshDirectoriesReplacesCheckFailure(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	failed := func(n *view.Needs) int {
		c := 0
		for _, it := range n.Items {
			if it.Title == view.T("directory check failed") {
				c++
			}
		}
		return c
	}
	db := e.App.Store.DB
	if _, err := db.Exec(`ALTER TABLE environment RENAME TO environment_away`); err != nil {
		t.Fatal(err)
	}
	n := view.RefreshDirectories(e.App, &view.Needs{})
	n = view.RefreshDirectories(e.App, n)
	if got := failed(n); got != 1 {
		t.Fatalf("after two failed checks: %d items", got)
	}
	if _, err := db.Exec(`ALTER TABLE environment_away RENAME TO environment`); err != nil {
		t.Fatal(err)
	}
	if n = view.RefreshDirectories(e.App, n); failed(n) != 0 {
		t.Fatalf("after recovery: %+v", n.Items)
	}
}
