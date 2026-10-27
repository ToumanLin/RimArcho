package gui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSortDependencyAPI(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	o, err := api.Run("sort")
	if err != nil || len(o.Refused) > 0 || len(o.MissingDependencies) != 1 {
		t.Fatalf("sort: %+v %v", o, err)
	}
	dep := o.MissingDependencies[0]
	if dep.Kind != "installed" || dep.Name != "Harmony" || strings.Join(dep.RequiredBy, ",") != "Alpha" {
		t.Fatalf("dep: %+v", dep)
	}
	raw, _ := json.Marshal(o)
	for _, field := range []string{`"missingDependencies"`, `"packageId"`, `"requiredBy"`, `"instance"`, `"ref"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("JSON lacks %s: %s", field, raw)
		}
	}
	before, _ := e.App.Loadout()
	r, err := api.CompleteDependencies()
	if err != nil || !r.Undo || len(r.Enabled) != 1 || len(r.MissingDependencies) != 0 || len(r.WorkshopIDs) != 0 {
		t.Fatalf("complete: %+v %v", r, err)
	}
	b := api.Board()
	if got := names(b.Cols[view.Active]); got != "Core,Harmony,Alpha" {
		t.Fatal(got)
	}
	if _, err := api.Undo(); err != nil {
		t.Fatal(err)
	}
	after, _ := e.App.Loadout()
	if !after.List.Equal(before.List) {
		t.Fatal("undo did not restore pre-completion state")
	}
}
