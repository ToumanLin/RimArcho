package view

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A player keeps an unofficial port on purpose while Use This Instead names the official
// Mod (player report, 1.0.9): the replacement card can be ignored, and the window after
// Sort stops suggesting it too.
func TestReplacedCardCanBeIgnored(t *testing.T) {
	t.Parallel()
	SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "fan.port", Name: "Unofficial port"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "fan.port")
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[{"oldPackageId":"fan.port","newPackageId":"author.official","newName":"Official","newWorkshopId":"2002","newVersions":["1.6"]}]}`)
	e.AddEnv(false)
	e.MustRun("scan")
	card := func() *Need {
		t.Helper()
		st, err := e.App.Loadout()
		if err != nil {
			t.Fatal(err)
		}
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range BuildNeeds(e.App, st, ps).Items {
			for _, a := range it.Actions {
				if a.Kind == ActIgnore {
					return &it
				}
			}
		}
		return nil
	}
	it := card()
	if it == nil || it.Title != "Unofficial port has a maintained replacement" {
		t.Fatalf("card %+v", it)
	}
	var ignore Action
	for _, a := range it.Actions {
		if a.Kind == ActIgnore {
			ignore = a
		}
	}
	if ignore.Key != "I" || ignore.Label != "ignore it" {
		t.Fatalf("ignore %+v", ignore)
	}
	if r, err := e.App.Sort(false); err != nil || len(r.Replacements) != 1 {
		t.Fatalf("sort before: %+v %v", r, err)
	}
	if done, err := Do(e.App, ignore, 0); err != nil || done != "ignored; shown again when Unofficial port changes" {
		t.Fatal(done, err)
	}
	if it := card(); it != nil {
		t.Fatalf("after ignore %+v", it)
	}
	if r, err := e.App.Sort(false); err != nil || len(r.Replacements) != 0 {
		t.Fatalf("sort after ignore: %+v %v", r.Replacements, err)
	}
	e.MustRun("problems", "unignore", ignore.Refs[0])
	if card() == nil {
		t.Fatal("unignore did not bring it back")
	}
}
