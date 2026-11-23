package view

import (
	"path/filepath"
	"strings"
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

// With the replacement installed, Space uses it in the old Mod's place as one step (16 §4).
// Two actions on Space (enable B, disable A) only ever enabled B and kept the card; with B
// named like A they read "enable MendAndRecycle / disable MendAndRecycle" (VM 2026-11-23).
func TestReplacedCardUsesTheReplacementInPlace(t *testing.T) {
	t.Parallel()
	SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "old.mend", Name: "MendAndRecycle"})
	e.AddWorkshop("2002", testkit.Mod{PackageID: "new.mend", Name: "MendAndRecycle"})
	e.AddWorkshop("3003", testkit.Mod{PackageID: "other.mod", Name: "Other"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "old.mend", "other.mod")
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[{"oldPackageId":"old.mend","newPackageId":"new.mend","newName":"MendAndRecycle","newWorkshopId":"2002","newVersions":["1.6"]}]}`)
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var use *Action
	for _, it := range BuildNeeds(e.App, st, ps).Items {
		if it.Title != "MendAndRecycle has a maintained replacement" {
			continue
		}
		var keys []string
		for i, a := range it.Actions {
			keys = append(keys, a.Key)
			if a.Kind == ActReplace {
				use = &it.Actions[i]
			}
		}
		if len(keys) != 2 || keys[0] != "space" || keys[1] != "I" {
			t.Fatalf("keys %v of %+v", keys, it.Actions)
		}
	}
	if use == nil || use.Label != "use MendAndRecycle instead" || !use.Undo {
		t.Fatalf("use %+v", use)
	}
	if _, err := Do(e.App, *use, 0); err != nil {
		t.Fatal(err)
	}
	out := e.MustRun("list", "--active")
	if i, j := strings.Index(out, "ws:2002"), strings.Index(out, "ws:3003"); i < 0 || j < 0 || i > j || strings.Contains(out, "ws:1001") {
		t.Fatalf("not replaced in place:\n%s", out)
	}
}
