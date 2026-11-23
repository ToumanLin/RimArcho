package app_test

import (
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A game-only first scan does not read the Workshop: the first scan that reads every source
// finds the library already installed, so nothing it finds is new (M7).
func TestNewSinceSkipsGameOnlyScans(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("5100", testkit.Sample("tester.old", "Old"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	if _, err := e.App.ScanWith(app.ScanOptions{GameOnly: true}); err != nil {
		t.Fatal(err)
	}
	e.Advance(time.Hour)
	scanNow(t, e)
	e.Advance(time.Hour)
	e.AddWorkshop("5200", testkit.Sample("tester.fresh", "Fresh"))
	scanNow(t, e)

	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	added, since := app.Added(st), e.App.NewSince(st.Ix.Env.GameID)
	got := map[string]bool{}
	for _, m := range st.Ix.Mods {
		got[m.Name] = app.IsNew(added[m.LineageID], since)
	}
	if got["Old"] || !got["Fresh"] {
		t.Fatalf("new marks = %v, want only Fresh", got)
	}
}
