package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// A test run loads and reaches the menu, a map run also starts a new game; neither loads
// a save or plays (08 §12).
func TestTestRunReaches(t *testing.T) {
	t.Parallel()
	at := func(names ...string) []logs.PhaseCount {
		var cs []logs.PhaseCount
		for _, n := range names {
			cs = logs.AddPhaseCount(cs, &logs.Phase{Name: n}, 1)
		}
		return cs
	}
	for _, c := range []struct {
		phases   []logs.PhaseCount
		map_, ok bool
	}{
		{nil, false, true}, {at("loading"), false, true}, {at("menu"), false, true}, {at("unknown"), false, true},
		{at("new"), false, false}, {at("new"), true, true}, {at("save", "play"), true, false}, {at("play", "loading"), false, true},
	} {
		if got := testRunReaches(c.phases, c.map_); got != c.ok {
			t.Errorf("%v map=%v: %v, want %v", c.phases, c.map_, got, c.ok)
		}
	}
}

func TestTestRunMissingFolders(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	game := filepath.Join(root, "game")
	workshop := filepath.Join(root, "workshop")
	ix := &Index{
		Env: &env.Environment{GameRoot: game}, ByID: map[string]*store.Mod{},
		Sources: []*source.Source{{Kind: source.Workshop, Root: workshop}},
	}
	st := &LoadoutState{Ix: ix}
	for i, path := range []string{
		filepath.Join(game, "Data", "Core"), filepath.Join(game, "Mods", "Local"),
		filepath.Join(workshop, "1001"), filepath.Join(game, "Mods", "nested", "Mod"),
		filepath.Join(root, "extra", "Linked"), filepath.Join(root, "extra", "WorkshopLink"),
		filepath.Join(root, "steamcmd", "1002"), filepath.Join(root, "absent"),
	} {
		id := fmt.Sprintf("mod%d", i)
		if i != 7 {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		ix.ByID[id] = &store.Mod{ID: id, PackageID: id, Name: id, RealPath: path}
		st.List = append(st.List, loadout.Entry{Instance: id})
	}
	if err := source.CreateLink(filepath.Join(game, "Mods", "link"), ix.ByID["mod4"].RealPath); err != nil {
		t.Fatal(err)
	}
	if err := source.CreateLink(filepath.Join(workshop, "1003"), ix.ByID["mod5"].RealPath); err != nil {
		t.Fatal(err)
	}
	ix.ByID["official"] = &store.Mod{ID: "official", Kind: source.Official}
	st.List = append(st.List, loadout.Entry{Instance: "official"}, loadout.Entry{Missing: "not.installed"})
	want := []TestRunMissing{
		{PackageID: "mod3", Name: "mod3"}, {PackageID: "mod5", Name: "mod5"},
		{PackageID: "mod6", Name: "mod6"}, {PackageID: "mod7", Name: "mod7"},
		{PackageID: "not.installed", Name: "not.installed"},
	}
	if got := testRunMissing(st); !reflect.DeepEqual(got, want) {
		t.Fatalf("missing %+v, want %+v", got, want)
	}
}
