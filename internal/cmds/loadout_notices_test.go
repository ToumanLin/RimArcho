package cmds_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestSortNoticesAndRepairCommands(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(true)
	for _, m := range []testkit.Mod{
		{PackageID: "tester.old", Name: "Old", Deps: []string{"tester.dep", "tester.remote", "tester.unknown"}},
		{PackageID: "tester.dep", Name: "Dependency", Deps: []string{"tester.transitive"}},
		{PackageID: "tester.transitive", Name: "Transitive"}, {PackageID: "tester.new", Name: "New"},
	} {
		e.WriteMod(filepath.Join(e.Game, "Mods", m.Name), m)
	}
	e.WriteMod(filepath.Join(e.SteamCMD, "installed"), testkit.Sample("tester.installed", "Installed"))
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.old", "tester.installed", "tester.absent")
	e.WriteFile(filepath.Join(e.Home, "cache", "steamDB.index.json"), `{"tester.remote":["123456","Remote"]}`)
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[{"oldPackageId":"tester.old","newPackageId":"tester.new"}]}`)
	e.MustRun("scan")
	before, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := e.App.Store.Workspace(before.Plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	head := e.App.Store.LastStepID(before.Plan.ID, "undo")
	var payload struct {
		app.SortNotices
		Applied bool `json:"applied"`
	}
	text := e.MustRun("sort", "--dry-run", "--json")
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(text, err)
	}
	want, err := e.App.Sort(true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(payload.SortNotices, want.SortNotices) || payload.Applied {
		t.Fatal("JSON differs from GUI source", text)
	}
	for _, s := range []string{"dependency installed:", "dependency workshop:", "dependency unknown:", "installed: true; can enable: true", "recommended replacement:"} {
		if !strings.Contains(e.MustRun("sort", "--dry-run"), s) {
			t.Fatal("missing notice", s)
		}
	}
	if len(payload.MissingMods) != 2 || len(payload.Replacements) != 1 {
		t.Fatal(text)
	}
	now, _ := e.App.Store.Workspace(before.Plan.ID)
	if now.Entries != stored.Entries || e.App.Store.LastStepID(before.Plan.ID, "undo") != head {
		t.Fatal("dry-run wrote")
	}
	for _, args := range [][]string{
		{"loadout", "complete-dependencies"}, {"loadout", "replace-recommended", "pkg:tester.old"}, {"loadout", "use-installed", "tester.installed"},
	} {
		e.MustRun(args...)
		if e.App.Store.LastStepID(before.Plan.ID, "undo") == head {
			t.Fatal("no edit", args)
		}
		e.MustRun("undo")
		after, _ := e.App.Store.Workspace(before.Plan.ID)
		if after.Entries != stored.Entries || e.App.Store.LastStepID(before.Plan.ID, "undo") != head {
			t.Fatal("not exactly one undo step", args)
		}
	}
	for _, args := range [][]string{
		{"loadout", "replace-recommended", "pkg:tester.old", "pkg:tester.dep"},
		{"loadout", "use-installed", "tester.installed", "tester.absent"},
	} {
		if out, code := e.Run(args...); code == 0 {
			t.Fatal("invalid batch accepted", out)
		}
		after, _ := e.App.Store.Workspace(before.Plan.ID)
		if after.Entries != stored.Entries || e.App.Store.LastStepID(before.Plan.ID, "undo") != head {
			t.Fatal("invalid batch partially applied", args)
		}
	}
	if err := json.Unmarshal([]byte(e.MustRun("sort", "--json")), &payload); err != nil || !payload.Applied {
		t.Fatal("applied JSON", err)
	}
}
