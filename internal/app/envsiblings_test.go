package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestEnvSiblingsAddCloneAndSwitch(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	original, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	main := original.Ix.Env
	// Include an edit newer than the last saved version.
	e.MustRun("disable", "pkg:tester.a")
	original, err = e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(e.UserData, "Config", "Prefs.xml"), "<Prefs>source</Prefs>")
	e.WriteFile(filepath.Join(e.UserData, "Config", "notes.txt"), "not a setting")
	e.WriteFile(filepath.Join(e.UserData, "HugsLib", "settings.xml"), "<settings/>")
	e.WriteFile(filepath.Join(e.UserData, "Saves", "keep.rws"), "not copied")
	args := `-popupwindow "-savedatafolder=some old folder" -quicktest`
	if err := e.App.Store.SetMeta("env:"+main.ID+":launch_args", args); err != nil {
		t.Fatal(err)
	}
	// Force the generated plan to choose a unique name.
	e.MustRun("plan", "new", "clone")
	var scans, instances, sources int
	count := func() []int {
		t.Helper()
		result := []int{}
		for _, table := range []string{"scan", "instance", "source"} {
			var n int
			if err := e.App.Store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
				t.Fatal(err)
			}
			result = append(result, n)
		}
		return result
	}
	counts := count()
	scans, instances, sources = counts[0], counts[1], counts[2]
	dest := filepath.Join(e.Root, "clone-data")
	e.WriteFile(filepath.Join(dest, "Config", "Prefs.xml"), "<Prefs>destination</Prefs>")
	out := e.MustRun("env", "add", "clone", "--like", "main", "--user-data", dest)
	if !strings.Contains(out, "left -savedatafolder out") {
		t.Fatal(out)
	}
	clone, err := e.App.Store.EnvByName("clone")
	if err != nil || clone.GameID != main.GameID {
		t.Fatalf("installation not reused: %+v %v", clone, err)
	}
	if got := e.App.LaunchArgs(clone.ID); got != "-popupwindow -quicktest" || e.App.LaunchArgs(main.ID) != args {
		t.Fatalf("args clone: %q", got)
	}
	if got := count(); !reflect.DeepEqual(got, []int{scans, instances, sources}) {
		t.Fatalf("clone scanned or duplicated inventory: %v", got)
	}
	e.MustRun("env", "use", "clone")
	cloned, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if cloned.Plan.ID == original.Plan.ID || cloned.Plan.Name != "clone 2" || !cloned.List.Equal(original.List) {
		t.Fatalf("clone workspace: %s %s", cloned.Plan.Name, cloned.List.Encode())
	}
	if e.App.Store.VersionCount(cloned.Plan.ID) != 1 || e.App.Store.Meta(store.StageKey(cloned.Plan.ID)) != "" {
		t.Fatal("history or staging copied")
	}
	b, err := os.ReadFile(filepath.Join(dest, "Config", "Prefs.xml"))
	if err != nil || string(b) != "<Prefs>destination</Prefs>" {
		t.Fatalf("destination overwritten: %s %v", b, err)
	}
	for _, p := range []string{filepath.Join(dest, "Config", "ModsConfig.xml"), filepath.Join(dest, "Config", "notes.txt"), filepath.Join(dest, "Saves")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("copied excluded path %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "HugsLib", "settings.xml")); err != nil {
		t.Fatal(err)
	}
	e.MustRun("env", "add", "empty", "--game", e.Game, "--saves", "own")
	empty, _ := e.App.Store.EnvByName("empty")
	before := e.ReadModsConfig()
	beforeLinks := linkNames(t, filepath.Join(e.Game, "Mods"))
	// Switching must work while the shared installation runs and do no physical work.
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42, ArgsKnown: true, SaveData: main.UserDataRoot} }
	e.MustRun("env", "use", "empty")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if st.Ix.Env.ID != empty.ID || st.Plan.ID == cloned.Plan.ID || len(st.List) == 0 {
		t.Fatalf("wrong environment/plan: %+v", st.Plan)
	}
	for _, en := range st.List {
		if m := st.Ix.ByID[en.Instance]; m == nil || m.Kind != source.Official {
			t.Fatalf("empty environment inherited a mod: %+v", en)
		}
	}
	e.MustRun("env", "use", "clone")
	st, err = e.App.Loadout()
	if err != nil || st.Ix.Env.ID != clone.ID || st.Plan.ID != cloned.Plan.ID {
		t.Fatalf("cache borrowed sibling context: %+v %v", st, err)
	}
	if !reflect.DeepEqual(count(), []int{scans, instances, sources}) || before != e.ReadModsConfig() || !reflect.DeepEqual(beforeLinks, linkNames(t, filepath.Join(e.Game, "Mods"))) {
		t.Fatal("sibling switch scanned, linked or wrote ModsConfig")
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	out = e.MustRun("env", "list")
	if strings.Count(out, "game: "+e.Game) != 1 || !strings.Contains(out, "env:empty") || !strings.Contains(out, "env:clone") {
		t.Fatalf("grouping: %s", out)
	}
	if _, _, err := e.App.EnvSet("empty", app.EnvSpec{UserDataRoot: clone.UserDataRoot}); err == nil {
		t.Fatal("set accepted duplicate pair")
	}
	if _, _, err := e.App.EnvAdd("nested", app.EnvSpec{GameRoot: e.Game, UserDataRoot: filepath.Join(dest, "nested")}); err == nil {
		t.Fatal("nested sibling root accepted")
	}
	// An ignore is local even when inventory and enabled list are identical.
	if err := e.App.Store.SetIgnore(main.ID, &store.Ignore{Code: "synthetic", Key: "same", CreatedAt: e.Now()}); err != nil {
		t.Fatal(err)
	}
	ignores, err := e.App.Store.Ignores(clone.ID)
	if err != nil || len(ignores) != 0 {
		t.Fatalf("sibling inherited ignore: %+v %v", ignores, err)
	}
	e.MustRun("env", "remove", "main", "--yes")
	if got := count(); got[1] != instances || got[2] != sources {
		t.Fatalf("nonlast removal deleted inventory: %v", got)
	}
	if st, err := e.App.Loadout(); err != nil || st.Ix.Env.ID != clone.ID {
		t.Fatalf("noncurrent removal changed context: %+v %v", st, err)
	}
}

func TestEnvSiblingMoveAndReattach(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "add", "sibling", "--game", e.Game, "--saves", "own")
	main, _ := e.App.Env()
	moved := filepath.Join(e.Root, "moved-game")
	e.CopyTree(e.Game, moved)
	out := e.MustRun("env", "set", "main", "--game", moved)
	sibling, _ := e.App.Store.EnvByName("sibling")
	if sibling.GameRoot != moved || sibling.GameID != main.GameID || !strings.Contains(out, "env:sibling") {
		t.Fatalf("move did not follow installation: %+v %s", sibling, out)
	}
	otherGame := secondGame(e, "other-game")
	e.MustRun("env", "add", "other", "--game", otherGame, "--saves", "own")
	other, _ := e.App.Store.EnvByName("other")
	e.MustRun("env", "set", "sibling", "--game", otherGame)
	sibling, _ = e.App.Store.EnvByName("sibling")
	main, _ = e.App.Store.EnvByName("main")
	if sibling.GameID != other.GameID || main.GameRoot != moved || main.GameID == sibling.GameID {
		t.Fatalf("reattach moved another environment: %+v %+v", main, sibling)
	}
}

func TestSiblingSaveRecoversInterruptedTestJournal(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	main, _ := e.App.Env()
	e.MustRun("env", "add", "sibling", "--game", e.Game, "--saves", "own")
	mods := filepath.Join(e.Game, "Mods")
	hidden := filepath.Join(e.Game, "_ArchoTestHidden", "restored")
	e.WriteFile(filepath.Join(hidden, "marker"), "restore")
	rec, _ := json.Marshal(map[string]any{"mods": mods, "links": []any{}, "hidden": []map[string]string{{"name": "restored", "to": hidden}}})
	journal := filepath.Join(e.Home, "tests", main.ID, "links.json")
	e.WriteFile(journal, string(rec))
	e.MustRun("env", "use", "sibling")
	if _, err := e.App.Save(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mods, "restored", "marker")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatalf("journal retained: %v", err)
	}
}

func TestSiblingPackVisibilityAndOrigin(t *testing.T) {
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("pack", "build", "shared-pack")
	e.MustRun("env", "add", "sibling", "--game", e.Game, "--saves", "own")
	e.MustRun("env", "use", "sibling")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	packs, _, err := e.App.PacksOf(st.Ix.Env)
	if err != nil || len(packs) != 1 || packs[0].Env != "main" {
		t.Fatalf("pack visibility/origin: %+v %v", packs, err)
	}
	if out := e.MustRun("pack", "list"); !strings.Contains(out, "env main") {
		t.Fatal(out)
	}
	p, err := e.App.ReadPack("shared-pack")
	if err != nil || p.Env != "main" {
		t.Fatalf("manifest origin changed: %+v %v", p, err)
	}
}
