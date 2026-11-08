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
// a save or plays (20 §2).
func TestTestRunReaches(t *testing.T) {
	t.Parallel()
	// The Monitor's boundaries: menu, new, play (after the new game), save "Colony", play.
	marks := []*logs.Phase{{Name: "menu", Index: 1}, {Name: "new", Index: 2}, {Name: "play", Index: 3},
		{Name: "save", Save: "Colony", Index: 4}, {Name: "play", Index: 5}}
	at := func(idx ...int) *LogCluster {
		lc := &LogCluster{Cluster: &logs.Cluster{First: &logs.Entry{Line: 1}}}
		for _, i := range idx {
			p := logs.LoadingPhase
			if i > 0 {
				p = marks[i-1]
			}
			lc.Phases = logs.AddPhaseCount(lc.Phases, p, 1)
		}
		return lc
	}
	menu, mapRun, ticks := testRunTarget{}, testRunTarget{mapRun: true}, testRunTarget{mapRun: true, ticks: true}
	colony, other := testRunTarget{save: "colony", ticks: true}, testRunTarget{save: "Other", ticks: true}
	for i, c := range []struct {
		lc     *LogCluster
		target testRunTarget
		ok     bool
	}{
		{at(), menu, true}, {at(0), menu, true}, {at(1), menu, true},
		{at(2), menu, false}, {at(2), mapRun, true}, {at(3), mapRun, false}, {at(3), ticks, true}, {at(3), colony, false},
		{at(4, 5), mapRun, false}, {at(4, 5), colony, true}, {at(5), colony, true}, {at(5), other, false}, {at(4), other, false},
		{at(5, 0), menu, true},
	} {
		if got := testRunReaches(c.lc, marks, nil, c.target); got != c.ok {
			t.Errorf("case %d: %v, want %v", i, got, c.ok)
		}
	}
	// Without Monitor phases the game's own lines place the first occurrence (h8).
	game := []*logs.Phase{{Name: "save", Save: "autostart", Line: 100}, {Name: "new", Line: 200}}
	first := func(line int) *LogCluster { return &LogCluster{Cluster: &logs.Cluster{First: &logs.Entry{Line: line}}} }
	for i, c := range []struct {
		line   int
		target testRunTarget
		ok     bool
	}{
		{50, menu, true}, {150, menu, false}, {150, mapRun, false}, {150, testRunTarget{save: "Autostart"}, true},
		{150, other, false}, {250, mapRun, true}, {250, menu, false}, {250, colony, false},
	} {
		if got := testRunReaches(first(c.line), nil, game, c.target); got != c.ok {
			t.Errorf("game case %d: %v, want %v", i, got, c.ok)
		}
	}
	if !testRunReaches(&LogCluster{Cluster: &logs.Cluster{First: &logs.Entry{Line: 150}}, OnlyCompanion: true}, nil, game, menu) {
		t.Error("a companion-only cluster has no Player.log line to place")
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
	if got := testRunMissing(st, nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("missing %+v, want %+v", got, want)
	}
}
