package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// A native Linux game and a Proton copy in another Steam library: the guide's cards, the
// Game Config rows, the switchers' installation label and the game folder row tell them
// apart (16 §2.8, §2.15).
func TestProtonBadge(t *testing.T) {
	defer view.SetLang("auto")
	view.SetLang("en")
	e := testkit.New(t)
	e.App.Detect.GOOS = "linux"
	proton, protonData := e.ProtonLibrary("SteamLibrary2")

	source := map[string]string{}
	for _, g := range view.FindGames(e.App) {
		source[g.Game] = g.Source
	}
	if source[e.Game] != "Steam" || source[proton] != "Proton" {
		t.Fatalf("guide chips: %v", source)
	}
	if g, err := view.GameAt(e.App, proton); err != nil || g.Source != "Proton" {
		t.Fatalf("typed Proton folder: %+v %v", g, err)
	}
	if g, err := view.GameAt(e.App, e.OtherGame("loose", "1.5.4409 rev1118")); err != nil || g.Source != "started directly" {
		t.Fatalf("typed folder outside Steam: %+v %v", g, err)
	}

	e.AddEnv(false)
	e.MustRun("env", "add", "proton", "--game", proton, "--user-data", protonData, "--launch", "steam")
	rows := map[string]view.EnvRow{}
	for _, r := range view.EnvironmentRows(e.App, "") {
		rows[r.Name] = r
	}
	if r := rows["proton"]; r.Build != "Proton" || r.Installation != "RimWorld 1.6  Proton" {
		t.Fatalf("Proton row: %+v", r)
	}
	if r := rows["main"]; r.Build != "" || r.Installation != "RimWorld 1.6" {
		t.Fatalf("native row: %+v", r)
	}

	e.MustRun("env", "use", "proton")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var note string
	for _, g := range view.EnvGroups(e.App, st) {
		for _, it := range g.Items {
			if it.ValuePath && it.Value == proton {
				note = it.Note
			}
		}
	}
	if !strings.HasSuffix(note, "  Proton") {
		t.Fatalf("game folder note: %q", note)
	}

	out := e.MustRun("env", "list")
	if strings.Count(out, "build: proton") != 1 {
		t.Fatalf("env list build lines:\n%s", out)
	}
}
