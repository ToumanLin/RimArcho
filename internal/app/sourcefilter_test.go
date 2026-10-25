package app_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestDirectorySourceIdentityAndCopyFilters(t *testing.T) {
	e := testkit.New(t)
	s := &source.Source{ID: "stable-folder-id", Label: "dev", Root: filepath.Join(e.Root, "Development"), Kind: source.Extra}
	before := app.DirectoryAlias(s)
	s.Root = filepath.Join(e.Root, "Renamed")
	if app.DirectoryAlias(s) != before || e.App.DirectoryRole(s) != "other" {
		t.Fatal("directory identity changed with its path")
	}
	s.Label = "local"
	if app.DirectoryAlias(s) != "folder/local" {
		t.Fatal("custom folder collides with a built-in role")
	}
	lib := &app.Library{Lineages: []*app.LibLineage{{ID: "one", Name: "One", Instances: []*app.LibInstance{
		{Mod: &store.Mod{}, Source: app.LibWorkshop, DirectoryValues: []string{"workshop"}, Content: store.HasDLL, Versions: []string{"1.6"}},
		{Mod: &store.Mod{}, Source: app.LibLocal, DirectoryValues: []string{"local"}, Versions: []string{"1.5"}},
	}}}}
	for _, include := range [][]string{{"source:local", "content:assembly"}, {"source:local", "version:1.6"}} {
		r, err := lib.Filter(app.LibQuery{Include: include})
		if err != nil || len(r.Matches) != 0 {
			t.Fatalf("mixed different copies: %+v %v", r, err)
		}
	}
	r, err := lib.Filter(app.LibQuery{Include: []string{"source:workshop", "content:assembly"}})
	if err != nil || len(r.Matches) != 1 {
		t.Fatalf("matching copy: %+v %v", r, err)
	}
	lib.Lineages[0].Tags = []string{"Facial Hair"}
	for _, query := range []struct {
		filter string
		count  int
	}{{"tag:Facial", 1}, {"tags:Facial", 0}, {"tags:Facial Hair", 1}} {
		r, err := lib.Filter(app.LibQuery{Include: []string{query.filter}})
		if err != nil || len(r.Matches) != query.count {
			t.Fatalf("%s: %+v %v", query.filter, r, err)
		}
	}
}
