package app_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestRimCrowProfilesAdoption(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	game := e.OtherGame("imported", "1.6.4633 rev1261")
	rootA, rootB := filepath.Join(e.Root, "Crow data", "a"), filepath.Join(e.Root, "Crow data", "b")
	config := `<?xml version="1.0"?><ModsConfigData><version>1.6.4633 rev1261</version><activeMods><li>ludeon.rimworld</li><li>missing.example</li></activeMods></ModsConfigData>`
	for _, root := range []string{rootA, rootB} {
		e.WriteFile(filepath.Join(root, "Config", "ModsConfig.xml"), config)
	}
	path := filepath.Join(e.Root, "profiles.db")
	argv := []string{"-title", "two words", "", `C:\folder with spaces\`, `a"b`}
	testkit.RimCrowProfiles(t, path, testkit.CrowProfile{ID: "a", Name: "Colony A", Game: game, UserData: rootA, Args: argv}, testkit.CrowProfile{ID: "b", Name: "Colony B", Game: game, UserData: rootB})
	original, _ := os.ReadFile(path)
	key := e.App.Store.ChangeKey()
	dry, err := e.App.ImportRimCrow(app.RimCrowOptions{DB: path, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Profiles) != 2 || dry.Profiles[0].Status != "ready" || dry.Profiles[1].Status != "ready" {
		t.Fatalf("preview: %+v", dry.Profiles)
	}
	if e.App.Store.ChangeKey() != key {
		t.Fatal("dry run changed stored state")
	}
	rep, err := e.App.ImportRimCrow(app.RimCrowOptions{DB: path})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Profiles[0].Status != "imported" || rep.Profiles[1].Status != "imported" {
		t.Fatalf("import: %+v", rep.Profiles)
	}
	a, _ := e.App.Store.EnvByName("Colony-A")
	b, _ := e.App.Store.EnvByName("Colony-B")
	if a == nil || b == nil || a.GameID != b.GameID || a.UserDataRoot != rootA || b.UserDataRoot != rootB {
		t.Fatalf("environments: %+v %+v", a, b)
	}
	if e.App.Store.CurrentPlanID(a.ID) == e.App.Store.CurrentPlanID(b.ID) {
		t.Fatal("profiles share their current plan")
	}
	var scans int
	if err := e.App.Store.DB.QueryRow("SELECT count(*) FROM scan WHERE game_id=?", a.GameID).Scan(&scans); err != nil {
		t.Fatal(err)
	}
	if scans != 1 {
		t.Fatalf("installation scanned %d times", scans)
	}
	got, err := app.ParseLaunchArgs(e.App.LaunchArgs(a.ID))
	if err != nil || !reflect.DeepEqual(got, argv) {
		t.Fatalf("argv: %#v %v", got, err)
	}
	e.App.EnvName = a.Name
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.List.Encode(), "missing.example") {
		t.Fatalf("missing entry lost: %s", st.List.Encode())
	}
	if st.Plan.Name != "Colony A" {
		t.Fatalf("profile list name: %s", st.Plan.Name)
	}
	// A player edit and renamed Game Config survive repeated import by source profile ID.
	e.MustRun("disable", "pkg:missing.example")
	e.MustRun("env", "set", a.Name, "--name", "renamed")
	before := e.App.Store.ChangeKey()
	again, err := e.App.ImportRimCrow(app.RimCrowOptions{DB: path})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(again.Profiles[0].Status, "already imported") || e.App.Store.ChangeKey() != before {
		t.Fatalf("repeat changed player data: %+v", again.Profiles)
	}
	after, _ := os.ReadFile(path)
	if !reflect.DeepEqual(original, after) {
		t.Fatal("source database changed")
	}
	for _, root := range []string{rootA, rootB} {
		after, _ := os.ReadFile(filepath.Join(root, "Config", "ModsConfig.xml"))
		if string(after) != config {
			t.Fatal("adopted Config changed during import")
		}
	}
}

func TestRimCrowProfileConflicts(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	root := filepath.Join(e.Root, "new data")
	e.WriteFile(filepath.Join(root, "Config", "ModsConfig.xml"), e.ReadModsConfig())
	path := filepath.Join(e.Root, "conflicts.db")
	testkit.RimCrowProfiles(t, path,
		testkit.CrowProfile{ID: "empty", Name: "empty"},
		testkit.CrowProfile{ID: "pair", Name: "pair", Game: e.Game, UserData: e.UserData},
		testkit.CrowProfile{ID: "args", Name: "args", Game: e.Game, UserData: root, Args: []string{"-savedatafolder=elsewhere"}},
		testkit.CrowProfile{ID: "missing", Name: "missing", Game: e.Game, UserData: filepath.Join(e.Root, "missing Config")},
		testkit.CrowProfile{ID: "name", Name: "main", Game: e.Game, UserData: root},
	)
	before := e.App.Store.ChangeKey()
	rep, err := e.App.ImportRimCrow(app.RimCrowOptions{DB: path})
	if err != nil {
		t.Fatal(err)
	}
	reasons := ""
	for _, p := range rep.Profiles {
		if !strings.HasPrefix(p.Status, "skipped:") {
			t.Fatalf("not skipped: %+v", p)
		}
		reasons += p.Status + "\n"
	}
	for _, want := range []string{"unconfigured", "already uses", "overrides", "Config/ModsConfig.xml", "name collision"} {
		if !strings.Contains(reasons, want) {
			t.Fatalf("no %s: %s", want, reasons)
		}
	}
	if e.App.Store.ChangeKey() != before {
		t.Fatal("conflicts changed store")
	}
}

func TestJSONLaunchArgumentsReachLauncher(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct", "--launch-args", `["-title","two words","","C:\\trailing\\","a\"b"]`)
	e.MustRun("disable", "pkg:gone.mod")
	expected, err := app.ParseLaunchArgs(e.App.LaunchArgs(mustEnv(t, e)))
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.App.Launch(true)
	if err != nil {
		t.Fatal(err)
	}
	args := result.Launch.Args
	if len(args) < len(expected) || !reflect.DeepEqual(args[len(args)-len(expected):], expected) {
		t.Fatalf("launch argv: %#v expected suffix %#v", args, expected)
	}
}

// 03 §6.4: a profile of a game that already has a Game Config previews the entries that
// game cannot resolve.
func TestRimCrowProfilePreviewKnownGame(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	root := filepath.Join(e.Root, "Crow data", "known")
	e.WriteFile(filepath.Join(root, "Config", "ModsConfig.xml"), `<?xml version="1.0"?><ModsConfigData><version>1.6.4633 rev1261</version><activeMods><li>ludeon.rimworld</li><li>missing.example</li></activeMods></ModsConfigData>`)
	path := filepath.Join(e.Root, "profiles.db")
	testkit.RimCrowProfiles(t, path, testkit.CrowProfile{ID: "k", Name: "Known", Game: e.Game, UserData: root})
	dry, err := e.App.ImportRimCrow(app.RimCrowOptions{DB: path, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Profiles) != 1 || dry.Profiles[0].Status != "ready" || !reflect.DeepEqual(dry.Profiles[0].Unresolved, []string{"missing.example"}) {
		t.Fatalf("preview: %+v", dry.Profiles)
	}
}
