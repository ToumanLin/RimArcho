package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

var errInjected = errors.New("injected failure")

func failAt(step string) func(string) error {
	return func(s string) error {
		if s == step {
			return errInjected
		}
		return nil
	}
}

// An interrupted profile import resumes on the next import, at every step (03 §6.4).
func TestRimCrowProfileImportRecovers(t *testing.T) {
	for _, step := range []string{"profile-mapped", "env-registered", "profile-registered", "profile-plan"} {
		t.Run(step, func(t *testing.T) {
			e := loadoutEnv(t)
			scanNow(t, e)
			game := e.OtherGame("imported", "1.6.4633 rev1261")
			root := filepath.Join(e.Root, "Crow data", "a")
			e.WriteFile(filepath.Join(root, "Config", "ModsConfig.xml"), `<?xml version="1.0"?><ModsConfigData><version>1.6.4633 rev1261</version><activeMods><li>ludeon.rimworld</li></activeMods></ModsConfigData>`)
			path := filepath.Join(e.Root, "profiles.db")
			argv := []string{"-title", "two words"}
			testkit.RimCrowProfiles(t, path, testkit.CrowProfile{ID: "a", Name: "Colony A", Game: game, UserData: root, Args: argv})
			app.SetTestFault(e.App, failAt(step))
			if _, err := e.App.ImportRimCrow(app.RimCrowOptions{DB: path}); !errors.Is(err, errInjected) {
				t.Fatalf("injected failure not reported: %v", err)
			}
			app.SetTestFault(e.App, nil)
			dry, err := e.App.ImportRimCrow(app.RimCrowOptions{DB: path, DryRun: true})
			if err != nil {
				t.Fatal(err)
			}
			if dry.Profiles[0].Status != "ready" {
				t.Fatalf("interrupted profile is not importable: %+v", dry.Profiles[0])
			}
			rep, err := e.App.ImportRimCrow(app.RimCrowOptions{DB: path})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Profiles[0].Status != "imported" {
				t.Fatalf("resume: %+v", rep.Profiles[0])
			}
			es, _ := e.App.Store.Envs()
			if len(es) != 2 {
				t.Fatalf("Game Configs after resume: %d", len(es))
			}
			a, _ := e.App.Store.EnvByName("Colony-A")
			if a == nil || a.UserDataRoot != root || e.App.Store.CurrentPlanID(a.ID) == 0 {
				t.Fatalf("resumed Game Config: %+v", a)
			}
			if got, err := app.ParseLaunchArgs(e.App.LaunchArgs(a.ID)); err != nil || !reflect.DeepEqual(got, argv) {
				t.Fatalf("argv: %#v %v", got, err)
			}
			again, err := e.App.ImportRimCrow(app.RimCrowOptions{DB: path, DryRun: true})
			if err != nil || !strings.Contains(again.Profiles[0].Status, "already imported") {
				t.Fatalf("completed import: %+v %v", again.Profiles, err)
			}
		})
	}
}

// A failed env add leaves no Game Config registered (03 §6.2).
func TestEnvAddRollsBackOnFailure(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	other := secondGame(e, "other-game")
	app.SetTestFault(e.App, failAt("env-registered"))
	for _, args := range [][]string{{"env", "add", "sibling", "--game", e.Game, "--saves", "own"}, {"env", "add", "other", "--game", other}} {
		if out, code := e.Run(args...); code == 0 || !strings.Contains(out, errInjected.Error()) {
			t.Fatalf("%v: %d %s", args, code, out)
		}
	}
	app.SetTestFault(e.App, nil)
	es, _ := e.App.Store.Envs()
	if len(es) != 1 || es[0].Name != "main" {
		t.Fatalf("failed add stayed registered: %+v", es)
	}
	var games int
	if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM game`).Scan(&games); err != nil || games != 1 {
		t.Fatalf("installation records left: %d %v", games, err)
	}
	e.MustRun("env", "add", "sibling", "--game", e.Game, "--saves", "own")
	e.MustRun("env", "add", "other", "--game", other)
}

// A failed add forgets the list it created for the new Game Config.
func TestEnvAddRollbackForgetsNewList(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	before, _ := e.App.Store.Plans()
	app.SetTestFault(e.App, failAt("env-plan"))
	if out, code := e.Run("env", "add", "sibling", "--game", e.Game, "--saves", "own", "--new-list"); code == 0 || !strings.Contains(out, errInjected.Error()) {
		t.Fatalf("%d %s", code, out)
	}
	app.SetTestFault(e.App, nil)
	after, _ := e.App.Store.Plans()
	if len(after) != len(before) {
		t.Fatalf("lists after a failed add: %d, before %d", len(after), len(before))
	}
	e.MustRun("env", "add", "sibling", "--game", e.Game, "--saves", "own", "--new-list")
	if now, _ := e.App.Store.Plans(); len(now) != len(before)+1 {
		t.Fatalf("lists after the retry: %d", len(now))
	}
}

// Removing the current Game Config selects the next one in the same transaction, and its
// per-Game Config meta values go with it.
func TestEnvRemoveSelectsNextAndForgetsMeta(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "add", "other", "--game", secondGame(e, "other-game"))
	main, _ := e.App.Env()
	keys := []string{"ui.columns:" + main.ID, "memory-estimate:" + main.ID + ":3", "subscriptions:check:main:workshop", "log.seen:" + main.ID}
	for _, k := range append(keys, "subscriptions:check:shared:git") {
		if err := e.App.Store.SetMeta(k, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.App.Store.DeleteEnv(main.ID); err != nil {
		t.Fatal(err)
	}
	cur, err := e.App.Store.CurrentEnv()
	if err != nil || cur == nil || cur.Name != "other" {
		t.Fatalf("current after removal: %+v %v", cur, err)
	}
	for _, k := range keys {
		if v := e.App.Store.Meta(k); v != "" {
			t.Fatalf("%s left behind", k)
		}
	}
	if e.App.Store.Meta("subscriptions:check:shared:git") == "" {
		t.Fatal("shared subscription check removed")
	}
}

// Reattaching the last Game Config of an installation says its inventory goes.
func TestEnvReattachPreviewNamesForgottenInventory(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "add", "sibling", "--game", e.Game, "--saves", "own")
	otherGame := secondGame(e, "other-game")
	e.MustRun("env", "add", "other", "--game", otherGame, "--saves", "own")
	lines, err := e.App.EnvGameChangePreview("sibling", otherGame)
	if err != nil || len(lines) != 1 {
		t.Fatalf("sibling reattach: %v %v", lines, err)
	}
	e.MustRun("env", "set", "sibling", "--game", otherGame)
	lines, err = e.App.EnvGameChangePreview("main", otherGame)
	if err != nil || len(lines) != 2 || !strings.Contains(lines[1], "no other Game Config uses "+e.Game) || !strings.Contains(lines[1], "Mod inventory records") {
		t.Fatalf("last reattach: %v %v", lines, err)
	}
	if out := e.MustRun("env", "set", "main", "--game", otherGame); !strings.Contains(out, "no other Game Config uses") {
		t.Fatalf("env set output: %s", out)
	}
}

// A launcher that fails puts Player.log and Player-prev.log back (03 §6.3).
func TestFailedLaunchKeepsPlayerLog(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "add", "own", "--like", "main", "--saves", "own")
	e.MustRun("env", "use", "own")
	e.MustRun("env", "set", "own", "--launch", "direct")
	en, _ := e.App.Env()
	cur, prev := filepath.Join(en.UserDataRoot, "Player.log"), filepath.Join(en.UserDataRoot, "Player-prev.log")
	e.WriteFile(cur, "previous run")
	e.WriteFile(prev, "older run")
	e.App.Launcher = func(string, []string, string) error { return errors.New("fake start failed") }
	if _, err := e.App.Launch(false); err == nil {
		t.Fatal("failed launch accepted")
	}
	for path, want := range map[string]string{cur: "previous run", prev: "older run"} {
		if b, err := os.ReadFile(path); err != nil || string(b) != want {
			t.Fatalf("%s after failed launch: %q %v", path, b, err)
		}
	}
	e.App.Launcher = func(string, []string, string) error { return nil }
	if _, err := e.App.Launch(false); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(prev); err != nil || string(b) != "previous run" {
		t.Fatalf("rotation: %q %v", b, err)
	}
	ents, _ := os.ReadDir(en.UserDataRoot)
	for _, de := range ents {
		if strings.Contains(de.Name(), "rimarcho-kept") {
			t.Fatalf("kept log left: %s", de.Name())
		}
	}
}

// Launch arguments 1.0.9 stored with an unpaired quote still launch, split on whitespace.
func TestLegacyUnpairedQuoteLaunchArgs(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	id := mustEnv(t, e)
	if err := e.App.Store.SetMeta("env:"+id+":launch_args", "-title it's -popupwindow"); err != nil {
		t.Fatal(err)
	}
	r, err := e.App.Launch(true)
	if err != nil {
		t.Fatal(err)
	}
	args := r.Launch.Args
	if want := []string{"-title", "it's", "-popupwindow"}; len(args) < 3 || !reflect.DeepEqual(args[len(args)-3:], want) {
		t.Fatalf("argv: %#v", args)
	}
	// Entering such text now is refused with how to change it.
	if err := e.App.SetLaunchArgs(id, "-title it's"); err == nil || !strings.Contains(err.Error(), "archo env set main --launch-args") {
		t.Fatalf("new unpaired quote: %v", err)
	}
}
