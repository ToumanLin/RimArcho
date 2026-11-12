package gui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/surface/gui"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestManagerPreviewApply(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("3000000001", testkit.Sample("tester.alpha", "Alpha"))
	e.AddEnv(false)
	e.MustRun("scan")
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	path := filepath.Join(e.Root, "RimSort", "dbs", "userRules.json")
	original := `{"rules":{"tester.alpha":{"loadAfter":{"ludeon.rimworld":{}}},"tester.absent":{"loadTop":{"value":true}}}}`
	e.WriteFile(path, original)
	p, err := api.PreviewManagerImport("rimsort-rules", path)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := e.App.Env()
	rules, _ := e.App.Store.Rules(env.ID)
	if len(rules) != 0 || !strings.Contains(p.Text, "1 new") || !strings.Contains(p.Text, "1 skipped") {
		t.Fatalf("preview wrote data or wrong counts: %+v %+v", rules, p)
	}
	if _, err := api.ApplyManagerImport(p.Request + 1); err == nil {
		t.Fatal("accepted another preview")
	}
	e.WriteFile(path, `{"rules":{}}`)
	if _, err := api.ApplyManagerImport(p.Request); err == nil {
		t.Fatal("accepted changed source")
	}
	e.WriteFile(path, original)
	p, err = api.PreviewManagerImport("rimsort-rules", path)
	if err != nil {
		t.Fatal(err)
	}

	if err := e.App.Store.SetMeta("test-preview-change", "changed"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ApplyManagerImport(p.Request); err == nil {
		t.Fatal("accepted changed RimArcho data")
	}
	p, err = api.PreviewManagerImport("rimsort-rules", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.ApplyManagerImport(p.Request); err != nil {
		t.Fatal(err)
	}
	rules, _ = e.App.Store.Rules(env.ID)
	if len(rules) != 1 {
		t.Fatalf("rules: %+v", rules)
	}
	p, err = api.PreviewManagerImport("rimsort-rules", path)
	if err != nil || !strings.Contains(p.Text, "1 already present") {
		t.Fatalf("repeat: %+v %v", p, err)
	}
	if b, _ := os.ReadFile(path); string(b) != original {
		t.Fatal("source changed")
	}
}

func TestRimCrowProfileConfigInvalidatesPreview(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	root := filepath.Join(e.Root, "profile")
	file := filepath.Join(root, "Config", "ModsConfig.xml")
	e.WriteFile(file, `<ModsConfigData><version>1.6</version><activeMods><li>ludeon.rimworld</li></activeMods></ModsConfigData>`)
	path := filepath.Join(e.Root, "profile.db")
	testkit.RimCrowProfiles(t, path, testkit.CrowProfile{ID: "p", Name: "profile", Game: e.Game, UserData: root})
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	p, err := api.PreviewManagerImport("rimcrow", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Text, "deleting the profile in RimCrow deletes its saves") {
		t.Fatal(p.Text)
	}
	e.WriteFile(file, `<ModsConfigData><version>1.6</version><activeMods><li>ludeon.rimworld</li><li>changed.mod</li></activeMods></ModsConfigData>`)
	if _, err := api.ApplyManagerImport(p.Request); err == nil {
		t.Fatal("accepted changed external Config")
	}
	if env, _ := e.App.Store.EnvByName("profile"); env != nil {
		t.Fatal("stale preview registered a Game Config")
	}
}

func TestAddEnvironmentSelectsCurrentPlan(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	current, _ := e.App.Loadout()
	e.MustRun("plan", "new", "editor", "--from", "current")
	plan, err := e.App.FindPlan("editor")
	if err != nil {
		t.Fatal(err)
	}
	e.App.OpenPlan = plan.ID
	e.MustRun("disable", "pkg:ludeon.rimworld")
	displayed, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	if _, err := api.AddEnvironment("copied", "main", true, ""); err != nil {
		t.Fatal(err)
	}
	e.App.OpenPlan = 0
	e.App.EnvName = "copied"
	copied, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if copied.Plan.ID != current.Plan.ID || copied.List.Equal(displayed.List) {
		t.Fatalf("copied another plan: %s vs %s", copied.List.Encode(), displayed.List.Encode())
	}
}

func TestRimCrowProfilesFirstRun(t *testing.T) {
	e := testkit.New(t)
	root := filepath.Join(e.Root, "first-profile")
	e.WriteFile(filepath.Join(root, "Config", "ModsConfig.xml"), `<ModsConfigData><version>1.6</version><activeMods><li>ludeon.rimworld</li></activeMods></ModsConfigData>`)
	path := filepath.Join(e.Root, "first.db")
	testkit.RimCrowProfiles(t, path, testkit.CrowProfile{ID: "p", Name: "first", Game: e.Game, UserData: root})
	text := e.MustRun("import", "rimcrow", "--db", path, "--dry-run")
	if !strings.Contains(text, "first → first: ready") {
		t.Fatal(text)
	}
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	preview, err := api.PreviewManagerImport("rimcrow", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.ApplyManagerImport(preview.Request); err != nil {
		t.Fatal(err)
	}
	current, err := e.App.Env()
	if err != nil || current.Name != "first" {
		t.Fatalf("current: %+v %v", current, err)
	}
}

func TestRimCrowProfileChecklistSelection(t *testing.T) {
	e := testkit.New(t)
	db := filepath.Join(e.Root, "selected.db")
	var profiles []testkit.CrowProfile
	for _, id := range []string{"a", "b"} {
		root := filepath.Join(e.Root, "profiles", id)
		e.WriteFile(filepath.Join(root, "Config", "ModsConfig.xml"), `<ModsConfigData><version>1.6</version><activeMods><li>ludeon.rimworld</li></activeMods></ModsConfigData>`)
		profiles = append(profiles, testkit.CrowProfile{ID: id, Name: id, Game: e.Game, UserData: root})
	}
	profiles = append(profiles, testkit.CrowProfile{ID: "unconfigured", Name: "not set up"})
	testkit.RimCrowProfiles(t, db, profiles...)
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	preview, err := api.PreviewManagerImport("rimcrow", db)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Profiles) != 3 {
		t.Fatalf("checklist: %+v", preview)
	}
	if strings.Contains(preview.Text, "not set up") || strings.Contains(preview.Text, "use_workshop_mods") {
		t.Fatalf("duplicate or technical profile text: %s", preview.Text)
	}
	if _, err = api.ApplyManagerImportProfiles(preview.Request, []string{"b"}); err != nil {
		t.Fatal(err)
	}
	es, err := e.App.Store.Envs()
	if err != nil || len(es) != 1 || es[0].Name != "b" {
		t.Fatalf("selection: %+v %v", es, err)
	}
	preview, err = api.PreviewManagerImport("rimcrow", db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = api.ApplyManagerImportProfiles(preview.Request, []string{}); err != nil {
		t.Fatal(err)
	}
	es, _ = e.App.Store.Envs()
	if len(es) != 1 {
		t.Fatal("empty selection imported configs")
	}
}
