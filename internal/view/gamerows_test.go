package view_test

import (
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The Game Config page's first section (16 §2.8): each game a Game Config plays, oldest
// version first, with the configs that use it; then a detected game no config uses.
func TestGameRows(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("env", "add", "modded", "--game", e.Game, "--saves", "own")
	old := e.OtherGame("old", "1.5.4409 rev1118")
	e.MustRun("env", "add", "old", "--game", old, "--user-data", e.UserData)
	spare := e.OtherGame("spare", "1.4.3901 rev1100")
	e.App.Detect.ManualRoots = append(e.App.Detect.ManualRoots, spare)

	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	rows := view.BuildEnvPage(e.App, st).Games
	if len(rows) != 3 {
		t.Fatalf("game rows: %+v", rows)
	}
	if r := rows[0]; r.Version != "1.5" || r.Path != old || !slices.Equal(r.Configs, []string{"old"}) || r.Current || r.Detected {
		t.Fatalf("1.5 row: %+v", r)
	}
	if r := rows[1]; r.Version != "1.6" || r.Path != e.Game || !slices.Equal(r.Configs, []string{"main", "modded"}) || !r.Current || r.Detected {
		t.Fatalf("1.6 row: %+v", r)
	}
	// detected and not added comes last, whatever its version
	if r := rows[2]; r.Version != "1.4" || r.Path != spare || len(r.Configs) != 0 || !r.Detected {
		t.Fatalf("detected row: %+v", r)
	}
}
